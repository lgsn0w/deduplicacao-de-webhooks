STRATEGY ?= none
FAULT ?= timeout
SEED ?= 1
EVENTS ?= 100
CONCURRENCY ?= 10
OUTPUT ?= results/reproduced-exp-a.csv
SUMMARY_OUTPUT ?= results/reproduced-exp-a-summary.csv
SMOKE_OUTPUT ?= /tmp/payment-reliability-smoke.csv
TRANSPORT ?= pooled
RECORD_TRANSPORT ?= false
DATABASE_URL ?= postgres://harness:harness@localhost:5433/harness?sslmode=disable
CONSUMER_URL ?= http://localhost:8082

.PHONY: test verify-results infra-up consumer cell-a summarize-exp-a docker-cell-a run-exp-a-min run-exp-a-full run-exp-a-sensitivity stop clean-results

test:
	go test -short ./...

verify-results:
	sha256sum -c RESULTS.sha256

infra-up:
	docker compose up -d postgres

consumer:
	STRATEGY=$(STRATEGY) DATABASE_URL=$(DATABASE_URL) go run ./cmd/consumer

cell-a:
	DATABASE_URL=$(DATABASE_URL) CONSUMER_URL=$(CONSUMER_URL) go run ./cmd/loadgen \
		--strategy=$(STRATEGY) \
		--fault=$(FAULT) \
		--seed=$(SEED) \
		--events=$(EVENTS) \
		--concurrency=$(CONCURRENCY) \
		--transport=$(TRANSPORT) \
		--record-transport=$(RECORD_TRANSPORT) \
		--output=$(OUTPUT)

summarize-exp-a:
	go run ./cmd/summarize --input=$(OUTPUT) --output=$(SUMMARY_OUTPUT)

docker-cell-a:
	STRATEGY=$(STRATEGY) docker compose up -d --build --force-recreate consumer
	sleep 2
	$(MAKE) cell-a STRATEGY=$(STRATEGY) FAULT=$(FAULT) SEED=$(SEED) EVENTS=$(EVENTS) CONCURRENCY=$(CONCURRENCY) TRANSPORT=$(TRANSPORT) RECORD_TRANSPORT=$(RECORD_TRANSPORT) OUTPUT=$(OUTPUT)

run-exp-a-min:
	$(RM) $(SMOKE_OUTPUT)
	$(MAKE) docker-cell-a STRATEGY=none FAULT=timeout SEED=1 OUTPUT=$(SMOKE_OUTPUT)
	$(MAKE) docker-cell-a STRATEGY=none FAULT=concurrent SEED=1 OUTPUT=$(SMOKE_OUTPUT)
	$(MAKE) docker-cell-a STRATEGY=dedup FAULT=timeout SEED=1 OUTPUT=$(SMOKE_OUTPUT)
	$(MAKE) docker-cell-a STRATEGY=dedup FAULT=concurrent SEED=1 OUTPUT=$(SMOKE_OUTPUT)

# Full Exp A: 9 cells × 5 seeds = 45 runs.
# timeout/concurrent use 1000 events; crash uses 100 (see README.md).
# 5s cooldown between cells per protocol.
run-exp-a-full:
	$(RM) $(OUTPUT)
	@for seed in 1 2 3 4 5; do \
		for strategy in none idem-key dedup; do \
			echo "=== $$strategy / timeout / seed=$$seed ===" ; \
			$(MAKE) docker-cell-a STRATEGY=$$strategy FAULT=timeout SEED=$$seed EVENTS=1000 ; \
			sleep 5 ; \
			echo "=== $$strategy / concurrent / seed=$$seed ===" ; \
			$(MAKE) docker-cell-a STRATEGY=$$strategy FAULT=concurrent SEED=$$seed EVENTS=1000 ; \
			sleep 5 ; \
			echo "=== $$strategy / crash / seed=$$seed ===" ; \
			$(MAKE) docker-cell-a STRATEGY=$$strategy FAULT=crash SEED=$$seed EVENTS=100 ; \
			sleep 5 ; \
		done ; \
	done
	$(MAKE) summarize-exp-a

# Controlled sensitivity: 2 transports × 2 strategies × 5 seeds = 20 runs.
# Each run uses 1000 event IDs × 10 concurrent deliveries = 10k requests.
run-exp-a-sensitivity:
	bash run-exp-a-sensitivity.sh

stop:
	docker compose down

clean-results:
	$(RM) $(OUTPUT)
