##@ License

LICENSEI_VERSION = 0.8.0

bin/licensei: bin/licensei-$(LICENSEI_VERSION)
	@ln -sf licensei-${LICENSEI_VERSION} bin/licensei

bin/licensei-$(LICENSEI_VERSION):
	@mkdir -p bin
	curl -sfL https://raw.githubusercontent.com/goph/licensei/master/install.sh | bash -s v$(LICENSEI_VERSION)
	@mv bin/licensei $@

.PHONY: license-check
license-check: bin/licensei ## Run license check
	@bin/licensei check
	@bin/licensei header

.PHONY: license-cache
license-cache: bin/licensei ## Generate license cache
	@bin/licensei cache
