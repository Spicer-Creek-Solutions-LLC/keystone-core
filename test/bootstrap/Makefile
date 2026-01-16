.PHONY: list-config test-scenarios test-platforms test-docker test-vm test-all

list-config:
	@echo "Bootstrap test config: test/bootstrap/config.yaml"

test-scenarios:
	KSCORE_BOOTSTRAP_TESTS=1 go test ./scenarios -timeout 30m

test-platforms:
	KSCORE_BOOTSTRAP_TESTS=1 go test ./platforms -timeout 30m

test-docker: test-scenarios test-platforms

test-vm:
	KSCORE_VM_TESTS=1 go test ./vm/... -timeout 2h

test-all: test-docker test-vm
