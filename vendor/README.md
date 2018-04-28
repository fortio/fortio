# vendor-fortio
Vendored dependencies for the fortio repo

Submodule for https://github.com/istio/fortio/

Updates here are made exclusively through
```
make depend.update DEPARGS="--update google.golang.org/grpc"
```

Similar (but smaller scope/simpler) than 
https://github.com/istio/vendor-istio/blob/master/README.md#how-do-i-add--change-a-dependency
