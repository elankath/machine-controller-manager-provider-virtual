.PHONY: build
build:
	go build -v -o bin/dev cmd/dev/main.go

.PHONY: setup
setup:
	@(printf "Project is: "; read PROJECT; printf "Shoot is: "; read SHOOT; ./bin/dev setup -project $$PROJECT -shoot $$SHOOT);

.PHONY: start
start:
	@GO111MODULE=on go run \
			cmd/machine-controller/main.go \
			--control-kubeconfig=$(CONTROL_KUBECONFIG) \
			--target-kubeconfig=$(TARGET_KUBECONFIG) \
			--namespace=$(CONTROL_NAMESPACE) \
			--machine-creation-timeout=20m \
			--machine-drain-timeout=5m \
			--machine-health-timeout=10m \
			--machine-pv-detach-timeout=2m \
			--machine-safety-apiserver-statuscheck-timeout=30s \
			--machine-safety-apiserver-statuscheck-period=1m \
			--machine-safety-orphan-vms-period=30m \
			--leader-elect=$(LEADER_ELECT) \
			--v=3

.PHONY: test
test:
	@(source gen/env; ./bin/dev start -all; cd test/integration/controller; ginkgo -v --show-node-events --poll-progress-after=300s --poll-progress-interval=60s)

.PHONY: clean
clean:
	./bin/dev stop -all || true
	@rm -rf ./gen/ || true
