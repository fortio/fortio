# Kubernetes ConfigMap example

- For production you would use docker and kubernetes. And use a ConfigMap mapped as a Volume in your service (see [dflag/configmap](../../configmap) for how/sample yaml files)
- For local testing you can use [Docker Desktop](https://www.docker.com/products/docker-desktop)
Or simply run from command line and simulate the changes
- initialize empty tmp mapping `mkdir -p /tmp/foobar` for kubernetes
- run the server `go run .`

you should see the following:
```
17:52:31 I updater.go:92> Now watching /tmp and /tmp/foobar
17:52:31 I updater.go:55> Configmap flag value watching initialized on /tmp/foobar
17:52:31 I updater.go:157> Starting watching
17:52:31 I http.go:58> Serving at: 0.0.0.0:8080
```

- see port you are serving
- visit the debug flag endpoint http://localhost:8080/debug/flags

Should see this if successful:
![screenshot](https://user-images.githubusercontent.com/3664595/88000279-1d225480-cab2-11ea-82ca-68658ad16148.png)

And you can update the flags right there in the URL -or- change a value in the configmap directory:
```
echo "456" > /tmp/foobar/example_my_dynamic_int
```
you will see in the logs:
```
17:56:24 I updater.go:151> updating example_my_dynamic_int to "456\n"
```

And the value is changed on the url as well (the `\n` is ignored during int parsing)
![changed value sshot](https://user-images.githubusercontent.com/3664595/88000485-99b53300-cab2-11ea-89d7-dcb5683bbc32.png)
