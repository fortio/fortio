# Locally test metrics with prometheus

local-prom:
<<<<<<< HEAD
	@echo "Starting local prometheus (^c to stop))"
	docker run -p 9090:9090 -v $(PWD)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus
=======
	@echo "Starting local prometheus"
	docker run --name prometheus -p 9090:9090 -v $(PWD)/prometheus.yml:/etc/prometheus/prometheus.yml prom/prometheus

clean:
	@echo "Stopping local prometheus"
	docker stop prometheus
	docker rm prometheus
>>>>>>> 90b3c5f (expose scli's num_fd and stats about currently running runs in prom endpoint)
