# Kubernetes ConfigMap example

- You need docker and kubernetes. [Docker Desktop](https://www.docker.com/products/docker-desktop), or you can use docker and [minikube](https://minikube.sigs.k8s.io/docs/start/)
- initialize empty tmp mapping `mkdir -p /tmp/foobar` for kubernetes
- run the server `go run .`

you should see the following:
```
server2020/07/15 13:14:08 Now watching /tmp and /tmp/foobar
server2020/07/15 13:14:08 etcd flag value watching initialized
server2020/07/15 13:14:08 starting watching
server2020/07/15 13:14:08 Serving at: 0.0.0.0:8080
```

- see port you are serving
- visit the debug flagz endpoint `http://0.0.0.0:8080/debug/flagz`

Should see this if successful:
![screenshot](https://raw.githubusercontent.com/ldemailly/go-flagz/screenshots/screenshot_endpoint.png)
