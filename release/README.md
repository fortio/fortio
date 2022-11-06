# How to make a fortio release

- All the builds and docker, except the build image updates, are now fully automated through github actions based on tags

- Make sure to use the same git tag format (e.g "v0.7.1" - note that there is `v` prefix in the tag, like many projects). Docker and internal version/tag is "0.7.1", the `v` is only for git tags.

- Once the release is deemed good/stable: move the git tag `latest_release` to the same as the release.

  ```Shell
  # for instance for 0.11.0:
  git fetch
  git checkout v0.11.0
  git tag -f latest_release
  git push -f --tags
  ```

- Also push `latest_release` docker tag/image:
  ```Shell
  go install github.com/regclient/regclient/cmd/regctl@latest
  regctl image copy fortio/fortio:1.30.0 fortio/fortio:latest_release
  ```

- To update the command line flags in the ../README.md; run `release/updateFlags.sh`

- Update the homebrew tap `brew bump-formula-pr --tag v1.2.3 fortio`


## How to change the build image

Update [../Dockerfile.build](../Dockerfile.build)

Edit the `BUILD_IMAGE_TAG := v50@sha...` line in the Makefile, set it to `v51`
for instance (replace `v50` by whichever is the next one at the time and temporarily remove the SHA part)

run

```Shell
make update-build-image
```

Make sure it gets successfully pushed to the fortio registry (requires org access)

Then do `make build-image-sha` to get the new image sha to replace/put in `BUILD_IMAGE_TAG` line of the Makefile

Then run

```Shell
make update-build-image-tag SED=gsed
```

Check the diff and make lint, webtest, etc and PR

Regularly update pinned github actions
using https://github.com/mheap/pin-github-action
```
npm install -g pin-github-action
```
for each action:
```
pin-github-action .github/workflows/...yml
```
