// Copyright © 2022 Cisco Systems, Inc. and/or its affiliates
// Copyright 2025 Adobe. All rights reserved.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package controllers

import (
	"context"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"time"

	"emperror.dev/errors"
	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	apiErrors "k8s.io/apimachinery/pkg/api/errors"
	v1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"

	"github.com/banzaicloud/go-cruise-control/pkg/types"

	apiutil "github.com/banzaicloud/koperator/api/util"
	banzaiv1alpha1 "github.com/banzaicloud/koperator/api/v1alpha1"
	banzaiv1beta1 "github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/cruisecontrol"
	"github.com/banzaicloud/koperator/pkg/scale"
	"github.com/banzaicloud/koperator/pkg/util"
)

const (
	defaultFailedTasksHistoryMaxLength             = 50
	ccOperationFinalizerGroup                      = "finalizer.cruisecontroloperations.kafka.banzaicloud.io"
	ccOperationForStopExecution                    = "ccOperationStopExecution"
	ccOperationFirstExecution                      = "ccOperationFirstExecution"
	ccOperationRetryExecution                      = "ccOperationRetryExecution"
	ccOperationInProgress                          = "ccOperationInProgress"
	defaultCruiseControlStatusOperationMaxDuration = time.Duration(5) * time.Minute
	// stalledCCDeploymentRequeueIntervalSeconds backs off the roll gate's requeue when the CC Deployment
	// rollout is wedged (ProgressDeadlineExceeded), so the error surfaced for it is not re-emitted every
	// default interval for the (potentially hours-long) life of the wedge.
	stalledCCDeploymentRequeueIntervalSeconds = 60
	// progressDeadlineExceededReason is the well-known reason the Deployment controller sets on the
	// Progressing condition when a rollout exceeds spec.progressDeadlineSeconds.
	progressDeadlineExceededReason = "ProgressDeadlineExceeded"
)

var (
	defaultRequeueIntervalInSeconds = 10
	executionPriorityMap            = map[banzaiv1alpha1.CruiseControlTaskOperation]int{
		banzaiv1alpha1.OperationAddBroker:    3,
		banzaiv1alpha1.OperationRemoveBroker: 2,
		banzaiv1alpha1.OperationRemoveDisks:  1,
		banzaiv1alpha1.OperationRebalance:    0,
	}
	missingCCResErr = errors.New("missing Cruise Control user task result")
)

// CruiseControlOperationReconciler reconciles CruiseControlOperation custom resources
type CruiseControlOperationReconciler struct {
	client.Client
	DirectClient client.Reader
	Scheme       *runtime.Scheme
	scaler       scale.CruiseControlScaler
	ScaleFactory func(ctx context.Context, kafkaCluster *banzaiv1beta1.KafkaCluster) (scale.CruiseControlScaler, error)
}

// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=cruisecontroloperations,verbs=get;list;watch;create;update;patch;delete;deletecollection
// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=cruisecontroloperations/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=kafka.banzaicloud.io,resources=cruisecontroloperations/finalizers,verbs=create;update;patch;delete
// The roll gate (requeueIfCCDeploymentNotRolledOut) reads the Cruise Control Deployment and ConfigMap; declare
// those reads locally so the controller's permissions stay correct even if the aggregated ClusterRole is ever
// split per-controller. These are a subset of what the KafkaCluster controller already grants, so regenerating
// the aggregated role produces no diff.
// +kubebuilder:rbac:groups=apps,resources=deployments,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=configmaps,verbs=get;list;watch

//nolint:gocyclo
func (r *CruiseControlOperationReconciler) Reconcile(ctx context.Context, request ctrl.Request) (ctrl.Result, error) {
	log := logr.FromContextOrDiscard(ctx)
	log.V(1).Info("reconciling CruiseControlOperation custom resources")

	ccOperationListClusterWide := banzaiv1alpha1.CruiseControlOperationList{}
	err := r.DirectClient.List(ctx, &ccOperationListClusterWide, client.ListOption(client.InNamespace(request.Namespace)))
	if err != nil {
		return requeueWithError(log, err.Error(), err)
	}

	// Selecting the reconciling CruiseControlOperation
	var currentCCOperation *banzaiv1alpha1.CruiseControlOperation
	for i := range ccOperationListClusterWide.Items {
		ccOperation := &ccOperationListClusterWide.Items[i]
		if ccOperation.GetName() == request.Name && ccOperation.GetNamespace() == request.Namespace {
			currentCCOperation = ccOperation
			break
		}
	}

	if currentCCOperation == nil {
		return reconciled()
	}

	// Skip reconciliation for Cruise Control Status operation
	if currentCCOperation.CurrentTaskOperation() == banzaiv1alpha1.OperationStatus {
		log.V(1).Info("skipping reconciliation for Cruise Control Status operation")
		return reconciled()
	}

	// When the task is done we can remove the finalizer instantly thus we can return fast here.
	if isFinalizerNeeded(currentCCOperation) && currentCCOperation.IsDone() {
		controllerutil.RemoveFinalizer(currentCCOperation, ccOperationFinalizerGroup)
		if err := r.Update(ctx, currentCCOperation); err != nil {
			return requeueWithError(log, "error happened when removing finalizer", err)
		}
		return reconciled()
	}

	// CruiseControlOperation has invalid operation type, reconciled
	if !currentCCOperation.IsCurrentTaskOperationValid() {
		log.Error(errors.New("Koperator does not support this operation"), "operation", currentCCOperation.CurrentTaskOperation())
		return reconciled()
	}

	kafkaClusterRef, err := kafkaClusterReference(currentCCOperation)
	if err != nil {
		return requeueWithError(log, "couldn't get kafka cluster reference", err)
	}

	kafkaCluster := &banzaiv1beta1.KafkaCluster{}
	err = r.Get(ctx, kafkaClusterRef, kafkaCluster)
	if err != nil {
		if apiErrors.IsNotFound(err) {
			if !currentCCOperation.DeletionTimestamp.IsZero() {
				log.Info("kafka cluster has been removed, now removing finalizer from CruiseControlOperation")
				controllerutil.RemoveFinalizer(currentCCOperation, ccOperationFinalizerGroup)
				if err := r.Update(ctx, currentCCOperation); err != nil {
					return requeueWithError(log, "failed to remove finalizer from CruiseControlOperation", err)
				}
			}
			return reconciled()
		}
		return requeueWithError(log, "failed to lookup referenced kafka cluster", err)
	}

	// Adding finalizer
	if err := r.addFinalizer(ctx, currentCCOperation); err != nil {
		return requeueWithError(log, "failed to add finalizer to CruiseControlOperation", err)
	}

	r.scaler, err = r.ScaleFactory(ctx, kafkaCluster)
	if err != nil {
		return requeueWithError(log, "failed to create Cruise Control Scaler instance", err)
	}

	// Checking Cruise Control health
	status, err := r.getStatus(ctx, log, kafkaCluster, kafkaClusterRef, ccOperationListClusterWide)
	if err != nil {
		log.Error(err, "could not get Cruise Control status")
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}

	if !status.IsReady() {
		log.Info("requeue event as Cruise Control is not ready (yet)", "status", status)
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}

	// Filtering out CruiseControlOperation by kafka cluster ref and state
	var ccOperationsKafkaClusterFiltered []*banzaiv1alpha1.CruiseControlOperation
	for i := range ccOperationListClusterWide.Items {
		operation := &ccOperationListClusterWide.Items[i]
		ref, err := kafkaClusterReference(operation)
		if err != nil {
			// Note: not returning here to continue processing the operations,
			// even if the user does not provide a KafkaClusterRef label on the CCOperation then the ref will be an empty object (not nil) and the filter will skip it.
			log.Info(err.Error())
		}
		if ref.Name == kafkaClusterRef.Name && ref.Namespace == kafkaClusterRef.Namespace &&
			operation.IsCurrentTaskOperationValid() && !operation.IsDone() {
			ccOperationsKafkaClusterFiltered = append(ccOperationsKafkaClusterFiltered, operation)
		}
	}

	// Update currentTask states from Cruise Control
	err = r.updateCurrentTasks(ctx, ccOperationsKafkaClusterFiltered)
	if err != nil {
		log.Error(err, "requeue event as updating state of currentTask(s) failed")
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}

	// When the task is not in execution we can remove the finalizer
	if isFinalizerNeeded(currentCCOperation) && !currentCCOperation.IsCurrentTaskRunning() {
		controllerutil.RemoveFinalizer(currentCCOperation, ccOperationFinalizerGroup)
		if err := r.Update(ctx, currentCCOperation); err != nil {
			return requeueWithError(log, err.Error(), err)
		}
		return reconciled()
	}

	// Sorting operations into categories which are sorted by priority
	ccOperationQueueMap := sortOperations(ccOperationsKafkaClusterFiltered)

	// When there is no more job present in the cluster we reconciled.
	if len(ccOperationQueueMap[ccOperationForStopExecution]) == 0 && len(ccOperationQueueMap[ccOperationFirstExecution]) == 0 &&
		len(ccOperationQueueMap[ccOperationRetryExecution]) == 0 && len(ccOperationQueueMap[ccOperationInProgress]) == 0 {
		log.Info("there is no more operation for execution")
		return reconciled()
	}

	ccOperationExecution, err := r.selectOperationForExecution(ccOperationQueueMap)
	if err != nil {
		log.Error(err, "requeue event as selecting operation for execution failed")
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}
	// There is nothing to be executed for now, requeue
	if ccOperationExecution == nil {
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}

	// Check if CruiseControl is ready as we cannot perform any operation until it is in ready state unless it is a stop execution operation
	if (status.InExecution() || len(ccOperationQueueMap[ccOperationInProgress]) > 0) && ccOperationExecution.CurrentTaskOperation() != banzaiv1alpha1.OperationStopExecution {
		// Requeue because we can't do more
		return requeueAfter(defaultRequeueIntervalInSeconds)
	}

	// Defer any Cruise Control operation (except stop_execution) until the CC Deployment is safe to submit to
	// - settled and carrying the current capacity.json - see requeueIfCCDeploymentNotRolledOut (#301).
	if result, handled, err := r.requeueIfCCDeploymentNotRolledOut(ctx, log, kafkaCluster,
		ccOperationExecution.CurrentTaskOperation(), ccOperationExecution.CurrentTaskParameters()); handled {
		return result, err
	}

	log.Info("executing Cruise Control task", "operation", ccOperationExecution.CurrentTaskOperation(), "parameters", ccOperationExecution.CurrentTaskParameters())
	// Executing operation
	cruseControlTaskResult, err := r.executeOperation(ctx, ccOperationExecution)

	if err != nil {
		log.Error(err, "Cruise Control task execution got an error", "name", ccOperationExecution.GetName(), "namespace", ccOperationExecution.GetNamespace(), "operation", ccOperationExecution.CurrentTaskOperation(), "parameters", ccOperationExecution.CurrentTaskParameters())
		// This can happen when the CruiseControlOperation parameter is wrong
		if cruseControlTaskResult == nil {
			return requeueWithError(log, "CruiseControlOperation custom resource is invalid", err)
		}
	}

	conflictRetryFunction := func() error {
		if err = updateResult(log, cruseControlTaskResult, ccOperationExecution, true); err != nil {
			return err
		}
		err = r.Status().Update(ctx, ccOperationExecution)
		if apiErrors.IsConflict(err) {
			err = r.Get(ctx, client.ObjectKey{Name: ccOperationExecution.GetName(), Namespace: ccOperationExecution.GetNamespace()}, ccOperationExecution)
		}
		return err
	}
	// Protection to avoid re-execution task when status update is failed
	if err := util.RetryOnConflict(util.DefaultBackOffForConflict, conflictRetryFunction); err != nil {
		return requeueWithError(log, "could not update the result of the Cruise Control user task execution to the CruiseControlOperation status", err)
	}

	return reconciled()
}

func (r *CruiseControlOperationReconciler) addFinalizer(ctx context.Context, currentCCOperation *banzaiv1alpha1.CruiseControlOperation) error {
	// examine DeletionTimestamp to determine if object is under deletion
	if currentCCOperation.DeletionTimestamp.IsZero() {
		// The object is not being deleted, so if it does not have our finalizer,
		// then lets add the finalizer and update the object. This is equivalent
		// registering our finalizer.
		if !controllerutil.ContainsFinalizer(currentCCOperation, ccOperationFinalizerGroup) {
			controllerutil.AddFinalizer(currentCCOperation, ccOperationFinalizerGroup)
			if err := r.Update(ctx, currentCCOperation); err != nil {
				return err
			}
		}
	}
	return nil
}

// requeueIfCCDeploymentNotRolledOut defers a Cruise Control operation until the CC Deployment is safe to
// submit to. Any broker-affecting op (add_broker, remove_broker, rebalance, remove_disks) submitted while the
// CC Deployment is mid-rollout can be wiped: during a RollingUpdate two CC pods briefly run behind one
// Service, so the fresh pod loses the in-memory task and resets its metric-sampling window, stalling the op
// (see #301). A roll is triggered by any change hashed into the CC pod template (capacity.json,
// cruisecontrol.properties, clusterConfigs.json, log4j.properties), so this is not specific to broker scaling.
// stop_execution is never gated - it is how an operator aborts a stuck task.
//
// "Settled right now" is not enough on its own: a ConfigMap change may not have been rolled into the pod
// template yet. KafkaClusterReconciler marks a new broker GracefulUpscaleRequired (which makes the task
// controller create the add_broker op) BEFORE, and independently of, the CC reconciler that regenerates the
// ConfigMap and patches the Deployment - so an op can be picked up while the Deployment is still settled on
// the OLD config, before the roll is even triggered. We therefore also require the running pod template's
// four hash annotations - stamped by cruisecontrol.GeneratePodAnnotations under CapacityConfigHashAnnotationKey,
// ConfigHashAnnotationKey, ClusterConfigHashAnnotationKey and LogConfigHashAnnotationKey - to each match the
// corresponding entry in the current ConfigMap for EVERY gated op (a rebalance/remove_disks would likewise be
// wiped by a roll triggered by any of capacity.json, cruisecontrol.properties, clusterConfigs.json or
// log4j.properties changing and landing just after submission - not just capacity). add_broker additionally
// requires capacity.json to contain the target broker(s), so it runs against a CC that has loaded their exact
// capacity (no dependency on capacity estimation). This is an implicit contract with the CC reconciler: both
// sides compute the same hash for each entry; keep them in sync (see GeneratePodAnnotations / TestCapacityConfigHash).
//
// Every check fails open when the evidence to gate on is absent (no Deployment, no ConfigMap, or a given
// hashed entry not koperator-managed) so it can never deadlock an operation. The returned bool reports
// whether the caller should return the (result, error) as-is.
func (r *CruiseControlOperationReconciler) requeueIfCCDeploymentNotRolledOut(ctx context.Context, log logr.Logger,
	kafkaCluster *banzaiv1beta1.KafkaCluster, op banzaiv1alpha1.CruiseControlTaskOperation, parameters map[string]string) (ctrl.Result, bool, error) {
	// stop_execution aborts an in-flight task and must never be deferred.
	if op == banzaiv1alpha1.OperationStopExecution {
		return ctrl.Result{}, false, nil
	}

	// Read the Deployment and ConfigMap through the non-cached API reader: freshness is part of this gate's
	// correctness contract. A lagging informer cache could show the OLD ConfigMap/Deployment annotations as
	// matching, let an op be submitted, and then have the real update land and roll Cruise Control - killing
	// the just-submitted task, the exact #301 race this gate prevents. This is a low-frequency path (only
	// selected non-stop_execution ops reach it), so the direct read is affordable. Fall back to the cached
	// client for manually constructed reconcilers (e.g. unit tests) that leave DirectClient unset.
	reader := r.DirectClient
	if reader == nil {
		reader = r.Client
	}

	deployment := &appsv1.Deployment{}
	deploymentKey := client.ObjectKey{
		Name:      cruisecontrol.DeploymentName(kafkaCluster),
		Namespace: kafkaCluster.Namespace,
	}
	if err := reader.Get(ctx, deploymentKey, deployment); err != nil {
		if apiErrors.IsNotFound(err) {
			// No Cruise Control Deployment: no rollout in progress to race with, so do not gate.
			return ctrl.Result{}, false, nil
		}
		result, wErr := requeueWithError(log, "could not determine Cruise Control Deployment rollout state", err)
		return result, true, wErr
	}

	// No op may be submitted to a CC that is mid-rollout - a restart wipes the in-flight task.
	if isDeploymentRolling(deployment) {
		requeueInterval := defaultRequeueIntervalInSeconds
		if deploymentRolloutTimedOut(deployment) {
			// The old CC pod keeps the Service up (so CC still reports ready) while the new pod never becomes
			// available - the operation would otherwise defer forever with only a routine log. Surface it, and
			// back off so a long-lived wedge does not re-emit this error every default interval.
			log.Error(errors.New("Cruise Control Deployment rollout exceeded its progress deadline"),
				"deferring Cruise Control operation on a wedged rollout - inspect the Cruise Control pod (image/crashloop); the operation stays deferred until the roll completes",
				"operation", op, "deployment", deployment.Name,
				"generation", deployment.Generation, "observedGeneration", deployment.Status.ObservedGeneration,
				"replicas", deployment.Status.Replicas, "updatedReplicas", deployment.Status.UpdatedReplicas)
			requeueInterval = stalledCCDeploymentRequeueIntervalSeconds
		} else {
			log.Info("requeue: Cruise Control Deployment is mid-rollout; deferring operation to avoid racing a CC restart",
				"operation", op, "deployment", deployment.Name,
				"generation", deployment.Generation, "observedGeneration", deployment.Status.ObservedGeneration,
				"replicas", deployment.Status.Replicas, "updatedReplicas", deployment.Status.UpdatedReplicas)
		}
		result, _ := requeueAfter(requeueInterval)
		return result, true, nil
	}

	// Pre-roll protection: the Deployment is settled, but a ConfigMap change may not have been rolled into the
	// pod template yet. This applies to EVERY gated op - a rebalance/remove_disks submitted now would also be
	// wiped by a roll (triggered by any of the four hashed entries below) that lands just after (see doc
	// comment). Each entry is checked independently: an absent annotation means that particular entry is not
	// koperator-managed on this Deployment (e.g. a custom capacity annotation, or a Deployment that predates
	// one of these annotations), so there is nothing to wait for on that axis specifically.
	configMap := &corev1.ConfigMap{}
	configMapKey := client.ObjectKey{
		Name:      cruisecontrol.ConfigMapName(kafkaCluster),
		Namespace: kafkaCluster.Namespace,
	}
	if err := reader.Get(ctx, configMapKey, configMap); err != nil {
		if apiErrors.IsNotFound(err) {
			// No Cruise Control ConfigMap to compare against yet; do not gate.
			return ctrl.Result{}, false, nil
		}
		result, wErr := requeueWithError(log, "could not read Cruise Control ConfigMap", err)
		return result, true, wErr
	}

	hashedConfigMapEntries := []struct {
		annotationKey string
		content       string
	}{
		{cruisecontrol.ConfigHashAnnotationKey, configMap.Data[cruisecontrol.PropertiesConfigMapKey]},
		{cruisecontrol.ClusterConfigHashAnnotationKey, configMap.Data[cruisecontrol.ClusterConfigsConfigMapKey]},
		{cruisecontrol.LogConfigHashAnnotationKey, configMap.Data[cruisecontrol.Log4jConfigMapKey]},
		{cruisecontrol.CapacityConfigHashAnnotationKey, configMap.Data[cruisecontrol.CapacityConfigMapKey]},
	}
	for _, entry := range hashedConfigMapEntries {
		deployedHash, ok := deployment.Spec.Template.Annotations[entry.annotationKey]
		if !ok {
			continue
		}
		if deployedHash != cruisecontrol.ConfigHash(entry.content) {
			log.Info("requeue: Cruise Control pod template does not yet carry the current ConfigMap; deferring operation until the roll settles",
				"operation", op, "annotation", entry.annotationKey,
				"deployedHash", deployedHash, "expectedHash", cruisecontrol.ConfigHash(entry.content),
				"generation", deployment.Generation, "observedGeneration", deployment.Status.ObservedGeneration)
			result, _ := requeueAfter(defaultRequeueIntervalInSeconds)
			return result, true, nil
		}
	}

	// Only add_broker additionally needs the target broker's capacity present before it runs.
	if op == banzaiv1alpha1.OperationAddBroker {
		brokerIDs := splitNonEmpty(parameters[scale.ParamBrokerID])
		hasAll, err := cruisecontrol.CapacityConfigContainsBrokers(configMap.Data[cruisecontrol.CapacityConfigMapKey], brokerIDs)
		if err != nil {
			result, wErr := requeueWithError(log, "could not inspect Cruise Control capacity config for the brokers being added", err)
			return result, true, wErr
		}
		if !hasAll {
			log.Info("requeue: capacity.json does not yet contain the broker(s) being added; deferring add_broker until their capacity is generated",
				"operation", op, "brokerIDs", brokerIDs, "configMap", configMapKey.Name, "deployment", deployment.Name)
			result, _ := requeueAfter(defaultRequeueIntervalInSeconds)
			return result, true, nil
		}
	}

	return ctrl.Result{}, false, nil
}

// splitNonEmpty splits a comma-separated parameter value, returning nil (rather than a single empty element)
// for an empty string so an absent parameter yields no broker ids.
func splitNonEmpty(s string) []string {
	if s == "" {
		return nil
	}
	return strings.Split(s, ",")
}

// isDeploymentRolling reports whether a Deployment is actively in the middle of a rollout: a new pod
// template has been applied but not yet observed by the Deployment controller, its pods have surged above
// the desired count, or not all running replicas are the latest revision yet. It deliberately keys off
// positive evidence of an in-progress rollout rather than "fully settled" so that a Deployment whose status
// has never been populated (observedGeneration == 0, e.g. under envtest where no Deployment controller
// runs) reads as NOT rolling and the check does not block. Initial CruiseControl availability is enforced
// separately by CruiseControlStatus.IsReady; this gate only guards against submitting a broker operation
// while an already-running CC is being re-rolled (e.g. by a capacity.json change).
func isDeploymentRolling(deployment *appsv1.Deployment) bool {
	specReplicas := int32(1)
	if deployment.Spec.Replicas != nil {
		specReplicas = *deployment.Spec.Replicas
	}
	s := deployment.Status
	switch {
	case s.ObservedGeneration != 0 && deployment.Generation > s.ObservedGeneration:
		// A new pod template was applied but the Deployment controller has not observed it yet.
		return true
	case s.Replicas > specReplicas:
		// RollingUpdate surge: an old-revision pod is still running alongside the new one.
		return true
	case s.UpdatedReplicas < s.Replicas:
		// Not all running pods are the latest revision yet.
		return true
	default:
		return false
	}
}

// deploymentRolloutTimedOut reports whether the Deployment controller has given up on the current rollout
// (Progressing=False with reason ProgressDeadlineExceeded), i.e. the new pod never became available - a
// wedged rollout the operator would otherwise defer a broker operation behind indefinitely.
func deploymentRolloutTimedOut(deployment *appsv1.Deployment) bool {
	for _, c := range deployment.Status.Conditions {
		if c.Type == appsv1.DeploymentProgressing && c.Reason == progressDeadlineExceededReason {
			return true
		}
	}
	return false
}

func (r *CruiseControlOperationReconciler) executeOperation(ctx context.Context, ccOperationExecution *banzaiv1alpha1.CruiseControlOperation) (*scale.Result, error) {
	var cruseControlTaskResult *scale.Result
	var err error
	switch ccOperationExecution.CurrentTaskOperation() {
	case banzaiv1alpha1.OperationAddBroker:
		cruseControlTaskResult, err = r.scaler.AddBrokersWithParams(ctx, ccOperationExecution.CurrentTaskParameters())
	case banzaiv1alpha1.OperationRemoveBroker:
		cruseControlTaskResult, err = r.scaler.RemoveBrokersWithParams(ctx, ccOperationExecution.CurrentTaskParameters())
	case banzaiv1alpha1.OperationRebalance:
		cruseControlTaskResult, err = r.scaler.RebalanceWithParams(ctx, ccOperationExecution.CurrentTaskParameters())
	case banzaiv1alpha1.OperationRemoveDisks:
		cruseControlTaskResult, err = r.scaler.RemoveDisksWithParams(ctx, ccOperationExecution.CurrentTaskParameters())
	case banzaiv1alpha1.OperationStopExecution:
		cruseControlTaskResult, err = r.scaler.StopExecution(ctx)
	case banzaiv1alpha1.OperationStatus:
		err = errors.NewWithDetails("Cruise Control operation not supported", "name", ccOperationExecution.GetName(), "namespace", ccOperationExecution.GetNamespace(), "operation", ccOperationExecution.CurrentTaskOperation(), "parameters", ccOperationExecution.CurrentTaskParameters())
	default:
		err = errors.NewWithDetails("Cruise Control operation not supported", "name", ccOperationExecution.GetName(), "namespace", ccOperationExecution.GetNamespace(), "operation", ccOperationExecution.CurrentTaskOperation(), "parameters", ccOperationExecution.CurrentTaskParameters())
	}
	return cruseControlTaskResult, err
}

func sortOperations(ccOperations []*banzaiv1alpha1.CruiseControlOperation) map[string][]*banzaiv1alpha1.CruiseControlOperation {
	ccOperationQueueMap := make(map[string][]*banzaiv1alpha1.CruiseControlOperation)
	for _, ccOperation := range ccOperations {
		switch {
		case isWaitingForFinalization(ccOperation):
			ccOperationQueueMap[ccOperationForStopExecution] = append(ccOperationQueueMap[ccOperationForStopExecution], ccOperation)
		case ccOperation.IsWaitingForFirstExecution():
			ccOperationQueueMap[ccOperationFirstExecution] = append(ccOperationQueueMap[ccOperationFirstExecution], ccOperation)
		case ccOperation.IsWaitingForRetryExecution():
			ccOperationQueueMap[ccOperationRetryExecution] = append(ccOperationQueueMap[ccOperationRetryExecution], ccOperation)
		case ccOperation.IsInProgress():
			ccOperationQueueMap[ccOperationInProgress] = append(ccOperationQueueMap[ccOperationInProgress], ccOperation)
		}
	}

	// Sorting by operation type and by the k8s object creation time
	for key := range ccOperationQueueMap {
		ccOperationQueue := ccOperationQueueMap[key]
		sort.SliceStable(ccOperationQueue, func(i, j int) bool {
			return executionPriorityMap[ccOperationQueue[i].CurrentTaskOperation()] > executionPriorityMap[ccOperationQueue[j].CurrentTaskOperation()] ||
				(executionPriorityMap[ccOperationQueue[i].CurrentTaskOperation()] == executionPriorityMap[ccOperationQueue[j].CurrentTaskOperation()] &&
					ccOperationQueue[i].CreationTimestamp.Unix() < ccOperationQueue[j].CreationTimestamp.Unix())
		})
	}
	return ccOperationQueueMap
}

// selectOperationForExecution selects the next operation to be executed
func (r *CruiseControlOperationReconciler) selectOperationForExecution(ccOperationQueueMap map[string][]*banzaiv1alpha1.CruiseControlOperation) (*banzaiv1alpha1.CruiseControlOperation, error) {
	// First prio: execute the finalize task
	if op := getFirstOperation(ccOperationQueueMap, ccOperationForStopExecution); op != nil {
		op.CurrentTask().Operation = banzaiv1alpha1.OperationStopExecution
		return op, nil
	}

	// Second prio: execute add_broker operation
	if op := getFirstOperation(ccOperationQueueMap, ccOperationFirstExecution); op != nil &&
		op.CurrentTaskOperation() == banzaiv1alpha1.OperationAddBroker {
		return op, nil
	}

	// Third prio: execute failed task
	if op := getFirstOperation(ccOperationQueueMap, ccOperationRetryExecution); op != nil {
		// If there is a failed remove_disks task and there is a rebalance_disks task in the queue, we execute the rebalance_disks task
		// This could only happen if the user tried to delete a disk, and later rolled back the change
		if op.CurrentTaskOperation() == banzaiv1alpha1.OperationRemoveDisks {
			for _, opFirstExecution := range ccOperationQueueMap[ccOperationFirstExecution] {
				if opFirstExecution.CurrentTaskOperation() == banzaiv1alpha1.OperationRebalance {
					// Mark the remove disk operation as paused, so it is not retried
					op.Labels[banzaiv1alpha1.PauseLabel] = True
					err := r.Update(context.TODO(), op)
					if err != nil {
						return nil, errors.WrapIfWithDetails(err, "failed to update Cruise Control operation", "name", op.Name, "namespace", op.Namespace)
					}

					// Execute the rebalance disks operation
					return opFirstExecution, nil
				}
			}
		}

		// When the default backoff duration elapsed we retry
		if op.IsReadyForRetryExecution() {
			return op, nil
		}
	}

	// Fourth prio: execute the first element in the FirstExecutionQueue which is ordered by operation type and k8s creation timestamp
	return getFirstOperation(ccOperationQueueMap, ccOperationFirstExecution), nil
}

// getFirstOperation returns the first operation in the given queue
func getFirstOperation(ccOperationQueueMap map[string][]*banzaiv1alpha1.CruiseControlOperation, key string) *banzaiv1alpha1.CruiseControlOperation {
	if len(ccOperationQueueMap[key]) > 0 {
		return ccOperationQueueMap[key][0]
	}
	return nil
}

// SetupCruiseControlWithManager registers cruise control controller to the manager
func SetupCruiseControlOperationWithManager(mgr ctrl.Manager) *ctrl.Builder {
	builder := ctrl.NewControllerManagedBy(mgr).
		For(&banzaiv1alpha1.CruiseControlOperation{}).
		WithEventFilter(SkipClusterRegistryOwnedResourcePredicate{}).
		Named("CruiseControlOperation")

	builder.WithEventFilter(
		predicate.Funcs{
			// We don't reconcile when there is no operation defined
			CreateFunc: func(e event.CreateEvent) bool {
				obj := e.Object.(*banzaiv1alpha1.CruiseControlOperation)
				// Doesn't need to reconcile when the operation is done and finalizing is not needed
				return (!obj.IsDone() || !obj.GetDeletionTimestamp().IsZero()) && obj.CurrentTaskOperation() != ""
			},
			UpdateFunc: func(e event.UpdateEvent) bool {
				oldObj := e.ObjectOld.(*banzaiv1alpha1.CruiseControlOperation)
				newObj := e.ObjectNew.(*banzaiv1alpha1.CruiseControlOperation)
				// Doesn't need to reconcile when the operation is done and finalizing is not needed
				if newObj.IsDone() && newObj.GetDeletionTimestamp().IsZero() {
					return false
				}
				if !reflect.DeepEqual(oldObj.CurrentTask(), newObj.CurrentTask()) ||
					oldObj.GetDeletionTimestamp() != newObj.GetDeletionTimestamp() ||
					oldObj.IsPaused() != newObj.IsPaused() ||
					oldObj.GetGeneration() != newObj.GetGeneration() {
					return true
				}
				return false
			},
		})

	return builder
}

func isFinalizerNeeded(operation *banzaiv1alpha1.CruiseControlOperation) bool {
	return controllerutil.ContainsFinalizer(operation, ccOperationFinalizerGroup) && !operation.DeletionTimestamp.IsZero()
}

func kafkaClusterReference(operation *banzaiv1alpha1.CruiseControlOperation) (client.ObjectKey, error) {
	if operation.GetClusterRef() == "" {
		return client.ObjectKey{}, errors.NewWithDetails("could not find kafka cluster reference label for CruiseControlOperation", "missing label", banzaiv1beta1.KafkaCRLabelKey, "name", operation.GetName(), "namespace", operation.GetNamespace())
	}
	return client.ObjectKey{
		Name:      operation.GetClusterRef(),
		Namespace: operation.GetNamespace(),
	}, nil
}

func updateResult(log logr.Logger, res *scale.Result, operation *banzaiv1alpha1.CruiseControlOperation, isAfterExecution bool) error {
	// This can happen rarely when the max cached completed user tasks is reached
	if res == nil {
		log.Error(missingCCResErr, "Cruise Control's max.cached.completed.user.tasks configuration value probably too small. Missing user task state is handled as completedWithError", "name", operation.GetName(), "namespace", operation.GetNamespace(), "task ID", operation.CurrentTaskID())
		res = &scale.Result{
			TaskID: operation.CurrentTaskID(),
			State:  banzaiv1beta1.CruiseControlTaskCompletedWithError,
			Err:    missingCCResErr,
		}
	}

	operation.Status.ErrorPolicy = operation.Spec.ErrorPolicy
	task := operation.CurrentTask()

	if (res.State == banzaiv1beta1.CruiseControlTaskCompleted || res.State == banzaiv1beta1.CruiseControlTaskCompletedWithError) && task.Finished == nil {
		task.Finished = &v1.Time{Time: time.Now()}
	}

	// Add the failed task into the status.failedTasks slice only when the update is happened after executing the task
	if isAfterExecution && task.Finished != nil && task.State == banzaiv1beta1.CruiseControlTaskCompletedWithError {
		if len(operation.Status.FailedTasks) >= defaultFailedTasksHistoryMaxLength {
			operation.Status.FailedTasks = operation.Status.FailedTasks[1:]
		}
		operation.Status.FailedTasks = append(operation.Status.FailedTasks, *task)

		operation.Status.RetryCount += 1
		task.SetDefaults()
	}

	if isAfterExecution {
		if task.Started == nil {
			startTime, err := time.Parse(time.RFC1123, res.StartedAt)
			if err != nil {
				return errors.WrapIff(err, "could not parse user task start time from Cruise Control API")
			}
			task.Started = &v1.Time{Time: startTime}
		}
		task.ID = res.TaskID
		task.Summary = formatSummary(res.Result)
		if res.Err != nil {
			task.ErrorMessage = res.Err.Error()
		}
		task.HTTPRequest = res.RequestURL
		task.HTTPResponseCode = &res.ResponseStatusCode
	}

	task.State = res.State

	return nil
}

// updateCurrentTasks the state of the CruiseControlOperation from the CruiseControlTasksAndStates instance by getting their
// status from Cruise Control.
func (r *CruiseControlOperationReconciler) updateCurrentTasks(ctx context.Context, ccOperations []*banzaiv1alpha1.CruiseControlOperation) error {
	log := logr.FromContextOrDiscard(ctx)

	userTaskIDs := make([]string, 0, len(ccOperations))
	var ccOperationsCopy []*banzaiv1alpha1.CruiseControlOperation
	for _, ccOperation := range ccOperations {
		if ccOperation.CurrentTaskID() != "" {
			userTaskIDs = append(userTaskIDs, ccOperation.CurrentTaskID())
		}

		ccOperationsCopy = append(ccOperationsCopy, ccOperation.DeepCopy())
	}

	tasks, err := r.scaler.UserTasks(ctx, userTaskIDs...)
	if err != nil {
		return errors.WrapIff(err, "could not get user tasks from Cruise Control API")
	}

	taskResultsByID := make(map[string]*scale.Result, len(tasks))
	for _, task := range tasks {
		taskResultsByID[task.TaskID] = task
	}
	for i := range ccOperations {
		ccOperation := ccOperations[i]
		if ccOperation.CurrentTaskID() != "" && !ccOperation.IsDone() {
			if err := updateResult(log, taskResultsByID[ccOperation.CurrentTaskID()], ccOperation, false); err != nil {
				return errors.WrapWithDetails(err, "could not set Cruise Control user task result to CruiseControlOperation CurrentTask", "name", ccOperations[i].GetName(), "namespace", ccOperations[i].GetNamespace())
			}
		}
	}

	for i := range ccOperations {
		if !reflect.DeepEqual(ccOperations[i].Status, ccOperationsCopy[i].Status) {
			if err := r.Status().Update(ctx, ccOperations[i]); err != nil {
				return errors.WrapIfWithDetails(err, "could not update CruiseControlOperation status", "name", ccOperations[i].GetName(), "namespace", ccOperations[i].GetNamespace())
			}
		}
	}
	return nil
}

// getStatus returns the internal state of Cruise Control.
//
// The logic is the following:
//   - If the Cruise Control makes the Status request sync, then the result will be returned.
//   - If the Cruise Control makes the Status request async, then a new Status CruiseControlOperation
//     will be created and an error will be returned to indicate that Cruise Control is not ready yet.
//   - If there is already a Status CruiseControlOperation in progress, then it will be updated and
//     the result will be returned.
func (r *CruiseControlOperationReconciler) getStatus(
	ctx context.Context,
	log logr.Logger,
	kafkaCluster *banzaiv1beta1.KafkaCluster,
	kafkaClusterRef client.ObjectKey,
	ccOperationListClusterWide banzaiv1alpha1.CruiseControlOperationList,
) (scale.CruiseControlStatus, error) {
	var statusOperation *banzaiv1alpha1.CruiseControlOperation
	for i := range ccOperationListClusterWide.Items {
		ccOperation := &ccOperationListClusterWide.Items[i]
		// ignoring the error here to continue processing the operations,
		// even if the user does not provide a KafkaClusterRef label on the CCOperation then the ref will be an empty object (not nil) and the filter will skip it.
		ref, _ := kafkaClusterReference(ccOperation)
		if ref.Name == kafkaClusterRef.Name && ref.Namespace == kafkaClusterRef.Namespace && ccOperation.Status.CurrentTask != nil &&
			ccOperation.Status.CurrentTask.Operation == banzaiv1alpha1.OperationStatus && ccOperation.IsCurrentTaskRunning() {
			statusOperation = ccOperation
			break
		}
	}

	if statusOperation != nil {
		res, err := r.scaler.StatusTask(ctx, statusOperation.CurrentTaskID())
		if err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not get the latest state of Status CruiseControlOperation", "name", statusOperation.GetName(), "namespace", statusOperation.GetNamespace())
		}
		if err := updateResult(log, res.TaskResult, statusOperation, false); err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not update the state of Status CruiseControlOperation", "name", statusOperation.GetName(), "namespace", statusOperation.GetNamespace())
		}

		err = r.Status().Update(ctx, statusOperation)
		if err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not update the state of Status CruiseControlOperation", "name", statusOperation.GetName(), "namespace", statusOperation.GetNamespace())
		}

		if statusOperation.CurrentTask().Finished != nil &&
			statusOperation.CurrentTask().Finished.Sub(statusOperation.CurrentTask().Started.Time) > defaultCruiseControlStatusOperationMaxDuration {
			return scale.CruiseControlStatus{}, errors.New("the Cruise Control status operation took too long to finish")
		}

		if res.Status == nil {
			return scale.CruiseControlStatus{}, errors.New("could not get Cruise Control status")
		}

		return *res.Status, nil
	}

	res, err := r.scaler.Status(ctx)
	if err != nil {
		return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not get Cruise Control status")
	}

	if res.Status == nil {
		operationTTLSecondsAfterFinished := kafkaCluster.Spec.CruiseControlConfig.CruiseControlOperationSpec.GetTTLSecondsAfterFinished()
		operation, err := r.createCCOperation(ctx, kafkaCluster, operationTTLSecondsAfterFinished, banzaiv1alpha1.OperationStatus)
		if err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not create a new Status CruiseControlOperation")
		}
		if err = updateResult(log, res.TaskResult, operation, true); err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not update the state of Status CruiseControlOperation", "name", operation.GetName(), "namespace", operation.GetNamespace())
		}
		err = r.Status().Update(ctx, operation)
		if err != nil {
			return scale.CruiseControlStatus{}, errors.WrapIfWithDetails(err, "could not update the state of Status CruiseControlOperation", "name", operation.GetName(), "namespace", operation.GetNamespace())
		}

		return scale.CruiseControlStatus{}, errors.New("could not get Cruise Control status, the operation is still in progress")
	}

	return *res.Status, nil
}

func (r *CruiseControlOperationReconciler) createCCOperation(
	ctx context.Context,
	kafkaCluster *banzaiv1beta1.KafkaCluster,
	ttlSecondsAfterFinished *int,
	operationType banzaiv1alpha1.CruiseControlTaskOperation,
) (*banzaiv1alpha1.CruiseControlOperation, error) {
	operation := &banzaiv1alpha1.CruiseControlOperation{
		ObjectMeta: v1.ObjectMeta{
			GenerateName: fmt.Sprintf("%s-%s-", kafkaCluster.Name, strings.ReplaceAll(string(operationType), "_", "")),
			Namespace:    kafkaCluster.Namespace,
			Labels:       apiutil.LabelsForKafka(kafkaCluster.Name),
		},
	}

	if ttlSecondsAfterFinished != nil {
		operation.Spec.TTLSecondsAfterFinished = ttlSecondsAfterFinished
	}

	if err := controllerutil.SetControllerReference(kafkaCluster, operation, r.Scheme); err != nil {
		return nil, err
	}
	if err := r.Create(ctx, operation); err != nil {
		return nil, err
	}

	operation.Status.CurrentTask = &banzaiv1alpha1.CruiseControlTask{
		Operation: operationType,
	}

	if err := r.Status().Update(ctx, operation); err != nil {
		return nil, err
	}

	return operation, nil
}

func isWaitingForFinalization(ccOperation *banzaiv1alpha1.CruiseControlOperation) bool {
	return ccOperation.IsCurrentTaskRunning() && !ccOperation.DeletionTimestamp.IsZero() && controllerutil.ContainsFinalizer(ccOperation, ccOperationFinalizerGroup)
}

func formatSummary(res *types.OptimizationResult) map[string]string {
	if res == nil {
		return nil
	}
	return map[string]string{
		"Data to move":                             fmt.Sprintf("%d", res.Summary.DataToMoveMB),
		"Number of replica movements":              fmt.Sprintf("%d", res.Summary.NumReplicaMovements),
		"Intra broker data to move":                fmt.Sprintf("%d", res.Summary.IntraBrokerDataToMoveMB),
		"Number of intra broker replica movements": fmt.Sprintf("%d", res.Summary.NumIntraBrokerReplicaMovements),
		"Number of leader movements":               fmt.Sprintf("%d", res.Summary.NumLeaderMovements),
		"Recent windows":                           fmt.Sprintf("%d", res.Summary.RecentWindows),
		"Provision recommendation":                 res.Summary.ProvisionRecommendation,
	}
}
