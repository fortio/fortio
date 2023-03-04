# Locally test metrics with prometheus

local-prom:
	@echo "Starting local prometheus"
	docker run --name prometheus -p 9090:9090 -v $(PWD)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus

clean:
	@echo "Stopping local prometheus"
	docker stop prometheus
	docker rm prometheus
