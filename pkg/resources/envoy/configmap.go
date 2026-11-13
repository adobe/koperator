// Copyright © 2019 Cisco Systems, Inc. and/or its affiliates
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

package envoy

import (
	"fmt"
	"math"
	"sort"

	"emperror.dev/errors"
	envoyaccesslog "github.com/envoyproxy/go-control-plane/envoy/config/accesslog/v3"
	envoybootstrap "github.com/envoyproxy/go-control-plane/envoy/config/bootstrap/v3"
	envoycluster "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	envoycore "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	envoyendpoint "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	envoylistener "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	envoyroute "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	envoyfileaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/file/v3"
	envoystdoutaccesslog "github.com/envoyproxy/go-control-plane/envoy/extensions/access_loggers/stream/v3"
	envoyhttphealthcheck "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/health_check/v3"
	envoyhttprouter "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	tls_inspectorv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/listener/tls_inspector/v3"
	envoyhcm "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	envoytcpproxy "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/tcp_proxy/v3"
	tlsv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/transport_sockets/tls/v3"
	envoytypesmatcher "github.com/envoyproxy/go-control-plane/envoy/type/matcher/v3"
	envoytypes "github.com/envoyproxy/go-control-plane/envoy/type/v3"
	"github.com/envoyproxy/go-control-plane/pkg/wellknown"
	"github.com/go-logr/logr"
	"sigs.k8s.io/yaml"

	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"

	"github.com/banzaicloud/koperator/api/v1beta1"
	"github.com/banzaicloud/koperator/pkg/resources/kafka"
	"github.com/banzaicloud/koperator/pkg/resources/templates"
	"github.com/banzaicloud/koperator/pkg/util"
	envoyutils "github.com/banzaicloud/koperator/pkg/util/envoy"
	kafkautils "github.com/banzaicloud/koperator/pkg/util/kafka"
)

// bindAddressAny is the wildcard address Envoy listeners and clusters bind to.
const bindAddressAny = "0.0.0.0"

// Runtime keys for the access-log samplers rendered when
// EnvoyConfig.EnableAccesslogSampling is set. Each is independently settable at
// runtime through the Envoy admin API, e.g.
//
//	curl -X POST 'localhost:8081/runtime_modify?access_log.broker_sample_rate=10000'
//
// Values are numerators over accessLogSampleDenominator, so 10000 is 100% and
// 0 (the rendered default) disables the log entirely.
const (
	// AccessLogRuntimeKeyBroker samples the per-broker TLS filter chains.
	AccessLogRuntimeKeyBroker = "access_log.broker_sample_rate"
	// AccessLogRuntimeKeyAnycast samples the anycast filter chain.
	AccessLogRuntimeKeyAnycast = "access_log.anycast_sample_rate"
	// AccessLogRuntimeKeyUnmatched samples the catch-all filter chain, which
	// receives every connection no more specific chain claimed.
	AccessLogRuntimeKeyUnmatched = "access_log.unmatched_sample_rate"
)

// accessLogSampleDenominator gives the runtime knobs a 0-10000 range, so a
// sample rate can be expressed to two decimal places (1 == 0.01%).
const accessLogSampleDenominator = envoytypes.FractionalPercent_TEN_THOUSAND

// Access-log line format, split around the chain name. These are deliberately
// concatenated rather than passed through fmt.Sprintf: Envoy's command
// operators (%START_TIME%, %DURATION%, ...) look like format verbs, and go vet
// fails the build with "unknown verb" if they reach a formatting function.
//
// The trailing newline is required - Envoy does not append one, so without it
// every log line would run into the next.
const (
	accessLogFormatPrefix = "[%START_TIME%] chain="
	accessLogFormatSuffix = " src=%DOWNSTREAM_REMOTE_ADDRESS% local=%DOWNSTREAM_LOCAL_ADDRESS%" +
		" sni=\"%REQUESTED_SERVER_NAME%\" bytes_recv=%BYTES_RECEIVED% bytes_sent=%BYTES_SENT%" +
		" duration=%DURATION% flags=%RESPONSE_FLAGS%\n"
)

// accessLogRuntimeLayerName is the name of the admin runtime layer. It is
// cosmetic to Envoy but appears in the /runtime admin endpoint output, where it
// tells an operator which layer a value came from.
const accessLogRuntimeLayerName = "admin"

// newAccessLogRuntimeLayer builds the bootstrap runtime layering that lets the
// access-log sampling keys be changed at runtime, e.g.
//
//	curl -X POST 'localhost:8081/runtime_modify?access_log.anycast_sample_rate=10000'
//
// A single admin layer is enough: it is writable through the admin API and
// overlays the values compiled into the config. Note that values set this way
// live only in the Envoy process, so they are lost on restart - the rendered
// config always comes back as "sample nothing".
func newAccessLogRuntimeLayer() *envoybootstrap.LayeredRuntime {
	return &envoybootstrap.LayeredRuntime{
		Layers: []*envoybootstrap.RuntimeLayer{{
			Name: accessLogRuntimeLayerName,
			// RuntimeLayer_AdminLayer_ (with the trailing underscore) is the
			// oneof wrapper; RuntimeLayer_AdminLayer is the message it holds.
			LayerSpecifier: &envoybootstrap.RuntimeLayer_AdminLayer_{
				AdminLayer: &envoybootstrap.RuntimeLayer_AdminLayer{},
			},
		}},
	}
}

// newRuntimeSampledAccessLog builds a file access log to /dev/stdout, gated by
// a runtime-settable sampling percentage, for the filter chain named chainName.
//
// The rendered sampler is inert: PercentSampled carries only a denominator, so
// the numerator defaults to 0 and nothing is logged until an operator raises
// runtimeKey through the admin API. Per Envoy's RuntimeFilter contract a value
// found at runtimeKey overrides PercentSampled, which is why the rendered
// config only ever needs to express the "off" fallback.
//
// chainName is embedded in every emitted line so the three chains stay
// distinguishable in a single stdout stream.
func newRuntimeSampledAccessLog(chainName, runtimeKey string) (*envoyaccesslog.AccessLog, error) {
	fileAccessLog := &envoyfileaccesslog.FileAccessLog{
		Path: "/dev/stdout",
		AccessLogFormat: &envoyfileaccesslog.FileAccessLog_LogFormat{
			LogFormat: &envoycore.SubstitutionFormatString{
				Format: &envoycore.SubstitutionFormatString_TextFormatSource{
					TextFormatSource: &envoycore.DataSource{
						Specifier: &envoycore.DataSource_InlineString{
							InlineString: accessLogFormatPrefix + chainName + accessLogFormatSuffix,
						},
					},
				},
			},
		},
	}

	pbstFileAccessLog, err := anypb.New(fileAccessLog)
	if err != nil {
		return nil, errors.WrapIf(err, "could not marshall envoy file access log config")
	}

	return &envoyaccesslog.AccessLog{
		Name: wellknown.FileAccessLog,
		Filter: &envoyaccesslog.AccessLogFilter{
			FilterSpecifier: &envoyaccesslog.AccessLogFilter_RuntimeFilter{
				RuntimeFilter: &envoyaccesslog.RuntimeFilter{
					RuntimeKey: runtimeKey,
					// Numerator is intentionally left unset: protojson omits
					// zero scalars, so this renders as a bare denominator and
					// means "sample nothing".
					PercentSampled: &envoytypes.FractionalPercent{
						Denominator: accessLogSampleDenominator,
					},
					// A documented no-op on TCP listeners (there is no
					// x-request-id to pivot on, which already forces
					// independent randomness), kept because it states the
					// intent explicitly and matches the config validated
					// against a live Envoy.
					UseIndependentRandomness: true,
				},
			},
		},
		ConfigType: &envoyaccesslog.AccessLog_TypedConfig{
			TypedConfig: pbstFileAccessLog,
		},
	}, nil
}

// brokerFilterChainName is the filter chain name used for a broker's chain, on
// both the TLS and the plaintext path. It deliberately differs from the broker
// cluster name ("broker-<id>") and from the TcpProxy stat prefix
// ("broker_tcp-<id>"): all three appear in Envoy output and keeping them
// distinct makes it obvious which one is being read.
func brokerFilterChainName(brokerId int) string {
	return fmt.Sprintf("broker_%d", brokerId)
}

// anycastFilterChainName names the chain serving the anycast hostname. The
// chain's stat prefix stays "all-brokers" for continuity with existing
// dashboards, so the two names differ on purpose.
const anycastFilterChainName = "anycast"

// attachAccessLogSampler adds the runtime-gated access log for chainName to
// tcpProxy, and is a no-op unless sampling is enabled.
//
// It has to run before the TcpProxy is marshalled into a filter chain, since
// marshalling snapshots the message. The caller must also set FilterChain.Name
// to the same chainName - the log line carries the chain name, so without it
// there is no way to trace a line back to the chain that emitted it.
//
// Both halves are gated on the same flag on purpose. Filter chain names are
// unset in koperator's default output, so naming them unconditionally would
// change the rendered config for every existing user.
func attachAccessLogSampler(tcpProxy *envoytcpproxy.TcpProxy, enabled bool, chainName, runtimeKey string) error {
	if !enabled {
		return nil
	}

	accessLog, err := newRuntimeSampledAccessLog(chainName, runtimeKey)
	if err != nil {
		return err
	}
	tcpProxy.AccessLog = []*envoyaccesslog.AccessLog{accessLog}

	return nil
}

func (r *Reconciler) configMap(log logr.Logger, extListener v1beta1.ExternalListenerConfig,
	ingressConfig v1beta1.IngressConfig, ingressConfigName, defaultIngressConfigName string) runtime.Object {
	eListenerLabelName := util.ConstructEListenerLabelName(ingressConfigName, extListener.Name)

	var configMapName = util.GenerateEnvoyResourceName(envoyutils.EnvoyVolumeAndConfigName, envoyutils.EnvoyVolumeAndConfigNameWithScope,
		extListener, ingressConfig, ingressConfigName, r.KafkaCluster.GetName())

	configMap := &corev1.ConfigMap{
		ObjectMeta: templates.ObjectMeta(
			configMapName,
			labelsForEnvoyIngress(r.KafkaCluster.GetName(), eListenerLabelName), r.KafkaCluster),
		Data: map[string]string{"envoy.yaml": GenerateEnvoyConfig(r.KafkaCluster, extListener, ingressConfig,
			ingressConfigName, defaultIngressConfigName, log)},
	}
	return configMap
}

func generateAddressValue(kc *v1beta1.KafkaCluster, brokerId int) string {
	if kc.Spec.HeadlessServiceEnabled {
		return fmt.Sprintf("%s-%d.%s-headless.%s.svc.%s", kc.Name, brokerId, kc.Name, kc.Namespace, kc.Spec.GetKubernetesClusterDomain())
	}
	//ClusterIP services are in use
	return fmt.Sprintf("%s-%d.%s.svc.%s", kc.Name, brokerId, kc.Namespace, kc.Spec.GetKubernetesClusterDomain())
}

func generateAnyCastAddressValue(kc *v1beta1.KafkaCluster) string {
	if kc.Spec.HeadlessServiceEnabled {
		return fmt.Sprintf("%s-headless.%s.svc.%s", kc.GetName(), kc.GetNamespace(), kc.Spec.GetKubernetesClusterDomain())
	}
	//ClusterIP services are in use
	return fmt.Sprintf(
		kafkautils.AllBrokerServiceTemplate+".%s.svc.%s", kc.GetName(), kc.GetNamespace(), kc.Spec.GetKubernetesClusterDomain())
}

func generateEnvoyHealthCheckListener(ingressConfig v1beta1.IngressConfig, log logr.Logger) *envoylistener.Listener {
	// health-check http listener
	stdoutAccessLog := &envoystdoutaccesslog.StdoutAccessLog{}
	pbstStdoutAccessLog, err := anypb.New(stdoutAccessLog)
	if err != nil {
		log.Error(err, "could not marshall envoy health-check stdoutAccessLog config")
		return nil
	}
	healthCheckConfig := &envoyhttphealthcheck.HealthCheck{
		PassThroughMode: wrapperspb.Bool(false),
		ClusterMinHealthyPercentages: map[string]*envoytypes.Percent{
			envoyutils.AllBrokerEnvoyConfigName: {Value: float64(1)},
		},
		Headers: []*envoyroute.HeaderMatcher{
			{
				Name: ":path",
				HeaderMatchSpecifier: &envoyroute.HeaderMatcher_StringMatch{
					StringMatch: &envoytypesmatcher.StringMatcher{
						IgnoreCase: true,
						MatchPattern: &envoytypesmatcher.StringMatcher_Exact{
							Exact: envoyutils.HealthCheckPath,
						},
					},
				},
			},
		},
	}
	pbstHealthCheckConfig, err := anypb.New(healthCheckConfig)
	if err != nil {
		log.Error(err, "could not marshall envoy healthCheckConfig config")
		return nil
	}

	httpHealthRouterConfig := &envoyhttprouter.Router{}
	pbstHttpHealthRouterConfig, err := anypb.New(httpHealthRouterConfig)
	if err != nil {
		log.Error(err, "could not marshall envoy stdoutAccessLog config")
		return nil
	}

	healthCheckFilter := &envoyhcm.HttpConnectionManager{
		StatPrefix: fmt.Sprintf("%s-healthcheck", envoyutils.AllBrokerEnvoyConfigName),
		RouteSpecifier: &envoyhcm.HttpConnectionManager_RouteConfig{
			RouteConfig: &envoyroute.RouteConfiguration{
				Name: "local",
				VirtualHosts: []*envoyroute.VirtualHost{
					{
						Name:    "localhost",
						Domains: []string{"*"},
						Routes: []*envoyroute.Route{
							{
								Match: &envoyroute.RouteMatch{
									PathSpecifier: &envoyroute.RouteMatch_Prefix{
										Prefix: "/",
									},
								},
								Action: &envoyroute.Route_Redirect{
									Redirect: &envoyroute.RedirectAction{
										PathRewriteSpecifier: &envoyroute.RedirectAction_PathRedirect{
											PathRedirect: envoyutils.HealthCheckPath,
										},
									},
								},
							},
						},
					},
				},
			},
		},
		HttpFilters: []*envoyhcm.HttpFilter{
			{
				Name: wellknown.HealthCheck,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: pbstHealthCheckConfig,
				},
			},
			{
				Name: wellknown.Router,
				ConfigType: &envoyhcm.HttpFilter_TypedConfig{
					TypedConfig: pbstHttpHealthRouterConfig,
				},
			},
		},
		AccessLog: []*envoyaccesslog.AccessLog{
			{
				Name: "envoy.access_loggers.stdout",
				ConfigType: &envoyaccesslog.AccessLog_TypedConfig{
					TypedConfig: pbstStdoutAccessLog,
				},
			},
		},
	}
	if ingressConfig.EnvoyConfig.EnableHealthCheckHttp10 {
		healthCheckFilter.HttpProtocolOptions = &envoycore.Http1ProtocolOptions{
			AcceptHttp_10: true,
		}
	}
	pbstHealthCheckFilter, err := anypb.New(healthCheckFilter)
	if err != nil {
		log.Error(err, "could not marshall envoy healthCheckFilter config")
		return nil
	}
	return &envoylistener.Listener{
		Address: &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: bindAddressAny,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: uint32(ingressConfig.EnvoyConfig.GetEnvoyHealthCheckPort()),
					},
				},
			},
		},
		FilterChains: []*envoylistener.FilterChain{
			{
				Filters: []*envoylistener.Filter{
					{
						Name: wellknown.HTTPConnectionManager,
						ConfigType: &envoylistener.Filter_TypedConfig{
							TypedConfig: pbstHealthCheckFilter,
						},
					},
				},
			},
		},
		SocketOptions: getKeepAliveSocketOptions(),
	}
}

func GenerateEnvoyTLSFilterChain(tcpProxy *envoytcpproxy.TcpProxy, brokerFqdn string, log logr.Logger) (*envoylistener.FilterChain, error) {
	tlsContext := &tlsv3.DownstreamTlsContext{
		CommonTlsContext: &tlsv3.CommonTlsContext{
			TlsParams: &tlsv3.TlsParameters{
				TlsMinimumProtocolVersion: tlsv3.TlsParameters_TLSv1_2,
				TlsMaximumProtocolVersion: tlsv3.TlsParameters_TLSv1_3,
			},
			TlsCertificates: []*tlsv3.TlsCertificate{
				{
					CertificateChain: &envoycore.DataSource{
						Specifier: &envoycore.DataSource_Filename{
							Filename: "/certs/certificate.crt",
						},
					},
					PrivateKey: &envoycore.DataSource{
						Specifier: &envoycore.DataSource_Filename{
							Filename: "/certs/private.key",
						},
					},
				},
			},
		},
	}
	pbTlsContext, err := anypb.New(tlsContext)
	if err != nil {
		log.Error(err, "could not marshall envoy tcp_proxy tls config")
		return nil, err
	}

	pbstTcpProxy, err := anypb.New(tcpProxy)
	if err != nil {
		log.Error(err, "could not marshall envoy tcp_proxy config")
		return nil, err
	}

	brokerTcpProxyFilter := &envoylistener.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoylistener.Filter_TypedConfig{
			TypedConfig: pbstTcpProxy,
		},
	}

	filterChain := &envoylistener.FilterChain{
		FilterChainMatch: &envoylistener.FilterChainMatch{
			ServerNames:       []string{brokerFqdn},
			TransportProtocol: "tls",
		},
		TransportSocket: &envoycore.TransportSocket{
			Name: "envoy.transport_sockets.tls",
			ConfigType: &envoycore.TransportSocket_TypedConfig{
				TypedConfig: pbTlsContext,
			},
		},
		Filters: []*envoylistener.Filter{
			brokerTcpProxyFilter,
		},
	}
	return filterChain, nil
}

func GenerateEnvoyFilterChain(tcpProxy *envoytcpproxy.TcpProxy, log logr.Logger) (*envoylistener.FilterChain, error) {
	pbstTcpProxy, err := anypb.New(tcpProxy)
	if err != nil {
		log.Error(err, "could not marshall envoy tcp_proxy config")
		return nil, err
	}

	brokerTcpProxyFilter := &envoylistener.Filter{
		Name: wellknown.TCPProxy,
		ConfigType: &envoylistener.Filter_TypedConfig{
			TypedConfig: pbstTcpProxy,
		},
	}

	filterChain := &envoylistener.FilterChain{
		Filters: []*envoylistener.Filter{
			brokerTcpProxyFilter,
		},
	}
	return filterChain, nil
}

// GenerateEnvoyConfig generate envoy configuration file
func GenerateEnvoyConfig(kc *v1beta1.KafkaCluster, elistener v1beta1.ExternalListenerConfig, ingressConfig v1beta1.IngressConfig,
	ingressConfigName, defaultIngressConfigName string, log logr.Logger) string {
	adminConfig := envoybootstrap.Admin{
		Address: &envoycore.Address{
			Address: &envoycore.Address_SocketAddress{
				SocketAddress: &envoycore.SocketAddress{
					Address: bindAddressAny,
					PortSpecifier: &envoycore.SocketAddress_PortValue{
						PortValue: uint32(ingressConfig.EnvoyConfig.GetEnvoyAdminPort()),
					},
				},
			},
		},
	}

	var listeners []*envoylistener.Listener
	var clusters []*envoycluster.Cluster
	var filterChain *envoylistener.FilterChain
	var err error

	tempListeners := make(map[int32][]*envoylistener.FilterChain)

	for _, brokerId := range util.GetBrokerIdsFromStatusAndSpec(kc.Status.BrokersState, kc.Spec.Brokers, log) {
		brokerConfig, err := kafkautils.GatherBrokerConfigIfAvailable(kc.Spec, brokerId)
		if err != nil {
			log.Error(err, "could not determine brokerConfig")
			continue
		}
		if util.ShouldIncludeBroker(brokerConfig, kc.Status, brokerId, defaultIngressConfigName, ingressConfigName) {
			// TCP_Proxy filter configuration
			tcpProxy := &envoytcpproxy.TcpProxy{
				StatPrefix:         fmt.Sprintf("broker_tcp-%d", brokerId),
				MaxConnectAttempts: &wrapperspb.UInt32Value{Value: 2},
				IdleTimeout:        &durationpb.Duration{Seconds: 560},
				ClusterSpecifier: &envoytcpproxy.TcpProxy_Cluster{
					Cluster: fmt.Sprintf("broker-%d", brokerId),
				},
			}

			brokerChainName := brokerFilterChainName(brokerId)
			if err := attachAccessLogSampler(tcpProxy, ingressConfig.EnvoyConfig.EnableAccesslogSampling,
				brokerChainName, AccessLogRuntimeKeyBroker); err != nil {
				log.Error(err, "Unable to generate broker envoy access log")
				return ""
			}

			if elistener.TLSEnabled() {
				filterChain, err = GenerateEnvoyTLSFilterChain(tcpProxy, func() string {
					// Broker IDs are always within valid range for int32 conversion
					if brokerId < 0 || brokerId > math.MaxInt32 {
						// This should never happen as broker IDs are small positive integers
						log.Error(fmt.Errorf("broker ID %d out of valid range for int32 conversion", brokerId), "Invalid broker ID detected in envoy TLS filter chain")
						return ""
					}
					return ingressConfig.EnvoyConfig.GetBrokerHostname(int32(brokerId))
				}(), log)
				if err != nil {
					log.Error(err, "Unable to generate broker envoy tls filter chain")
					return ""
				}
			} else {
				filterChain, err = GenerateEnvoyFilterChain(tcpProxy, log)
				if err != nil {
					log.Error(err, "Unable to generate broker envoy filter chain")
					return ""
				}
			}

			if ingressConfig.EnvoyConfig.EnableAccesslogSampling {
				filterChain.Name = brokerChainName
			}

			brokerPort := func() int32 {
				// Broker IDs are always within valid range for int32 conversion
				if brokerId < 0 || brokerId > math.MaxInt32 {
					// This should never happen as broker IDs are small positive integers
					log.Error(fmt.Errorf("broker ID %d out of valid range for int32 conversion", brokerId), "Invalid broker ID detected in envoy broker port assignment")
					return 0
				}
				return elistener.GetBrokerPort(int32(brokerId))
			}()
			tempListeners[brokerPort] = append(tempListeners[brokerPort], filterChain)

			clusters = append(clusters, &envoycluster.Cluster{
				Name:           fmt.Sprintf("broker-%d", brokerId),
				ConnectTimeout: &durationpb.Duration{Seconds: 1},
				UpstreamConnectionOptions: &envoycluster.UpstreamConnectionOptions{
					TcpKeepalive: getTcpKeepalive(),
				},
				ClusterDiscoveryType: &envoycluster.Cluster_Type{Type: envoycluster.Cluster_STRICT_DNS},
				LbPolicy:             envoycluster.Cluster_ROUND_ROBIN,
				// disable circuit breaking:
				// https://www.envoyproxy.io/docs/envoy/latest/faq/load_balancing/disable_circuit_breaking
				CircuitBreakers: &envoycluster.CircuitBreakers{
					Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
						{
							Priority:           envoycore.RoutingPriority_DEFAULT,
							MaxConnections:     &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxPendingRequests: &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxRequests:        &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxRetries:         &wrapperspb.UInt32Value{Value: 1_000_000_000},
						},
						{
							Priority:           envoycore.RoutingPriority_HIGH,
							MaxConnections:     &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxPendingRequests: &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxRequests:        &wrapperspb.UInt32Value{Value: 1_000_000_000},
							MaxRetries:         &wrapperspb.UInt32Value{Value: 1_000_000_000},
						},
					},
				},
				LoadAssignment: &envoyendpoint.ClusterLoadAssignment{
					ClusterName: fmt.Sprintf("broker-%d", brokerId),
					Endpoints: []*envoyendpoint.LocalityLbEndpoints{{
						LbEndpoints: []*envoyendpoint.LbEndpoint{{
							HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
								Endpoint: &envoyendpoint.Endpoint{
									Address: &envoycore.Address{
										Address: &envoycore.Address_SocketAddress{
											SocketAddress: &envoycore.SocketAddress{
												Protocol: envoycore.SocketAddress_TCP,
												Address:  generateAddressValue(kc, brokerId),
												PortSpecifier: &envoycore.SocketAddress_PortValue{
													PortValue: uint32(elistener.ContainerPort),
												},
											},
										},
									},
								},
							},
						}},
					}},
				},
			})
		}
	}

	// TCP_Proxy filter configuration
	tcpProxy := &envoytcpproxy.TcpProxy{
		StatPrefix:         envoyutils.AllBrokerEnvoyConfigName,
		IdleTimeout:        &durationpb.Duration{Seconds: 560},
		MaxConnectAttempts: &wrapperspb.UInt32Value{Value: 2},
		ClusterSpecifier: &envoytcpproxy.TcpProxy_Cluster{
			Cluster: envoyutils.AllBrokerEnvoyConfigName,
		},
	}

	if err := attachAccessLogSampler(tcpProxy, ingressConfig.EnvoyConfig.EnableAccesslogSampling,
		anycastFilterChainName, AccessLogRuntimeKeyAnycast); err != nil {
		log.Error(err, "Unable to generate anycast envoy access log")
		return ""
	}

	// Create TLS anycast broker listener
	if elistener.TLSEnabled() {
		filterChain, err = GenerateEnvoyTLSFilterChain(tcpProxy, ingressConfig.HostnameOverride, log)
		if err != nil {
			log.Error(err, "Unable to generate anycast envoy tls filter chain")
			return ""
		}
	} else {
		filterChain, err = GenerateEnvoyFilterChain(tcpProxy, log)
		if err != nil {
			log.Error(err, "Unable to generate anycast envoy filter chain")
			return ""
		}
	}

	if ingressConfig.EnvoyConfig.EnableAccesslogSampling {
		filterChain.Name = anycastFilterChainName
	}

	tempListeners[elistener.GetAnyCastPort()] = append(tempListeners[elistener.GetAnyCastPort()], filterChain)

	// sort the tempListeners map for consistent results
	ports := make([]int, 0, len(tempListeners))
	for p := range tempListeners {
		ports = append(ports, int(p))
	}
	sort.Ints(ports)

	tlsListenerFilter := &tls_inspectorv3.TlsInspector{}
	pbTlsListenerFilter, _ := anypb.New(tlsListenerFilter)

	for _, p := range ports {
		newListener := &envoylistener.Listener{
			Address: &envoycore.Address{
				Address: &envoycore.Address_SocketAddress{
					SocketAddress: &envoycore.SocketAddress{
						Address: bindAddressAny,
						PortSpecifier: &envoycore.SocketAddress_PortValue{
							PortValue: uint32(p),
						},
					},
				},
			},
			FilterChains:  tempListeners[int32(p)],
			SocketOptions: getKeepAliveSocketOptions(),
		}

		if elistener.TLSEnabled() {
			newListener.ListenerFilters = []*envoylistener.ListenerFilter{
				{
					Name: "tls_inspector",
					ConfigType: &envoylistener.ListenerFilter_TypedConfig{
						TypedConfig: pbTlsListenerFilter,
					},
				},
			}
		}
		listeners = append(listeners, newListener)
	}

	// health-check http listener
	healthCheckListener := generateEnvoyHealthCheckListener(ingressConfig, log)
	if healthCheckListener == nil {
		return ""
	}
	listeners = append(listeners, healthCheckListener)

	clusters = append(clusters, &envoycluster.Cluster{
		Name:           envoyutils.AllBrokerEnvoyConfigName,
		ConnectTimeout: &durationpb.Duration{Seconds: 1},
		UpstreamConnectionOptions: &envoycluster.UpstreamConnectionOptions{
			TcpKeepalive: getTcpKeepalive(),
		},
		IgnoreHealthOnHostRemoval: true,
		HealthChecks: []*envoycore.HealthCheck{
			{
				Interval:           &durationpb.Duration{Seconds: 5},
				Timeout:            &durationpb.Duration{Seconds: 1},
				NoTrafficInterval:  &durationpb.Duration{Seconds: 5},
				UnhealthyInterval:  &durationpb.Duration{Seconds: 2},
				IntervalJitter:     &durationpb.Duration{Seconds: 1},
				UnhealthyThreshold: wrapperspb.UInt32(2),
				HealthyThreshold:   wrapperspb.UInt32(1),
				EventLogPath:       "/dev/stdout",
				HealthChecker: &envoycore.HealthCheck_HttpHealthCheck_{
					HttpHealthCheck: &envoycore.HealthCheck_HttpHealthCheck{
						Path: kafka.MetricsHealthCheck,
					},
				},
			},
		},
		ClusterDiscoveryType: &envoycluster.Cluster_Type{Type: envoycluster.Cluster_STRICT_DNS},
		LbPolicy:             envoycluster.Cluster_ROUND_ROBIN,
		// disable circuit breakingL:
		// https://www.envoyproxy.io/docs/envoy/latest/faq/load_balancing/disable_circuit_breaking
		CircuitBreakers: &envoycluster.CircuitBreakers{
			Thresholds: []*envoycluster.CircuitBreakers_Thresholds{
				{
					Priority:           envoycore.RoutingPriority_DEFAULT,
					MaxConnections:     &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxPendingRequests: &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxRequests:        &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxRetries:         &wrapperspb.UInt32Value{Value: 1_000_000_000},
				},
				{
					Priority:           envoycore.RoutingPriority_HIGH,
					MaxConnections:     &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxPendingRequests: &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxRequests:        &wrapperspb.UInt32Value{Value: 1_000_000_000},
					MaxRetries:         &wrapperspb.UInt32Value{Value: 1_000_000_000},
				},
			},
		},
		LoadAssignment: &envoyendpoint.ClusterLoadAssignment{
			ClusterName: envoyutils.AllBrokerEnvoyConfigName,
			Endpoints: []*envoyendpoint.LocalityLbEndpoints{{
				LbEndpoints: []*envoyendpoint.LbEndpoint{{
					HostIdentifier: &envoyendpoint.LbEndpoint_Endpoint{
						Endpoint: &envoyendpoint.Endpoint{
							HealthCheckConfig: &envoyendpoint.Endpoint_HealthCheckConfig{
								PortValue: uint32(kafka.MetricsPort),
							},
							Address: &envoycore.Address{
								Address: &envoycore.Address_SocketAddress{
									SocketAddress: &envoycore.SocketAddress{
										Protocol: envoycore.SocketAddress_TCP,
										Address:  generateAnyCastAddressValue(kc),
										PortSpecifier: &envoycore.SocketAddress_PortValue{
											PortValue: uint32(elistener.ContainerPort),
										},
									},
								},
							},
						},
					},
				}},
			}},
		},
	})

	config := envoybootstrap.Bootstrap_StaticResources{
		Listeners: listeners,
		Clusters:  clusters,
	}
	generatedConfig := envoybootstrap.Bootstrap{
		Admin:           &adminConfig,
		StaticResources: &config,
	}

	// The admin runtime layer is what makes the access-log knobs settable at
	// all: /runtime_modify needs a writable layer to put values in. Without it
	// the sampling keys would be stuck at their rendered default forever.
	if ingressConfig.EnvoyConfig.EnableAccesslogSampling {
		generatedConfig.LayeredRuntime = newAccessLogRuntimeLayer()
	}

	marshaller := &protojson.MarshalOptions{}
	marshalledProtobufConfig, err := marshaller.Marshal(&generatedConfig)
	if err != nil {
		log.Error(err, "could not marshall envoy config")
		return ""
	}

	marshalledConfig, err := yaml.JSONToYAML(marshalledProtobufConfig)
	if err != nil {
		log.Error(err, "could not convert config from Json to Yaml")
		return ""
	}
	return string(marshalledConfig)
}

func getTcpKeepalive() *envoycore.TcpKeepalive {
	return &envoycore.TcpKeepalive{
		KeepaliveProbes:   wrapperspb.UInt32(3),
		KeepaliveTime:     wrapperspb.UInt32(30),
		KeepaliveInterval: wrapperspb.UInt32(30),
	}
}

func getKeepAliveSocketOptions() []*envoycore.SocketOption {
	return []*envoycore.SocketOption{
		// enable socket keep-alive
		{
			// SOL_SOCKET = 1
			Level: 1,
			// SO_KEEPALIVE = 9
			Name:  9,
			Value: &envoycore.SocketOption_IntValue{IntValue: 1},
			State: envoycore.SocketOption_STATE_PREBIND,
		},
		// configure keep alive idle, interval and count
		{
			// IPPROTO_TCP = 6
			Level: 6,
			// TCP_KEEPIDLE = 4
			Name:  4,
			Value: &envoycore.SocketOption_IntValue{IntValue: 30},
			State: envoycore.SocketOption_STATE_PREBIND,
		},
		{
			// IPPROTO_TCP = 6
			Level: 6,
			// TCP_KEEPINTVL = 5
			Name:  5,
			Value: &envoycore.SocketOption_IntValue{IntValue: 30},
			State: envoycore.SocketOption_STATE_PREBIND,
		},
		{
			// IPPROTO_TCP = 6
			Level: 6,
			// TCP_KEEPCNT = 6
			Name:  6,
			Value: &envoycore.SocketOption_IntValue{IntValue: 3},
			State: envoycore.SocketOption_STATE_PREBIND,
		},
	}
}
