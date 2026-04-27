.PHONY: help networking-build networking-run control-plane-test control-plane-run microservices-build microservices-run automation-test smoke-test http-udm-smoke-suite compose-up compose-down compose-logs compose-refresh-mocks

help:
	@echo "AUSF polyglot workspace"
	@echo "Targets:"
	@echo "  make networking-build   Build the C++ PFCP layer"
	@echo "  make networking-run     Run the C++ PFCP sample"
	@echo "  make control-plane-test Run Java unit tests"
	@echo "  make control-plane-run  Start the Java control-plane service"
	@echo "  make microservices-build Build the Go AUSF service"
	@echo "  make microservices-run  Start the Go AUSF service"
	@echo "  make automation-test    Run Python automation tests"
	@echo "  make smoke-test         Run Python end-to-end smoke client"
	@echo "  make http-udm-smoke-suite Run the full HTTP UDM happy/negative smoke suite"
	@echo "  make compose-up         Start Go and Java services in Docker"
	@echo "  make compose-down       Stop Docker Compose stack"
	@echo "  make compose-refresh-mocks Restart mock-amf and mock-nrf reliably"
	@echo "  make compose-logs       Follow Docker Compose logs"

networking-build:
	cmake -S networking -B networking/build
	cmake --build networking/build

networking-run: networking-build
	./networking/build/Debug/networking_test.exe

control-plane-test:
	cd control-plane && mvn test

control-plane-run:
	cd control-plane && mvn spring-boot:run

microservices-build:
	cd microservices && go build ./...

microservices-run:
	cd microservices && go run ./cmd/ausf

automation-test:
	cd automation && python -m unittest discover -s tests

smoke-test:
	cd automation && python scripts/smoke_test.py

http-udm-smoke-suite:
	python automation/scripts/run_http_udm_smoke_suite.py

compose-up:
	docker compose up --build -d

compose-down:
	docker compose down

compose-refresh-mocks:
	pwsh -File automation/scripts/refresh_mock_services.ps1

compose-logs:
	docker compose logs -f