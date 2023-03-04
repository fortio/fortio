# Locally test metrics with prometheus

local-prom:
	@echo "Starting local prometheus (^c to stop))"
	docker run -p 9090:9090 -v $(PWD)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
