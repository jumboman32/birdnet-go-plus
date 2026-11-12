# Deployment

The fork's deployment material is grouped here:

- [Docker](docker/): Compose files, container startup scripts, and smoke tests.
- [Podman](podman/README.md): Compose and Quadlet configurations.
- [Release packages](release/): installation notes shipped with platform binaries.

The image recipe remains at the repository root as `Dockerfile`. From that
directory, build this fork's image with:

```bash
docker build -t birdnet-go-plus:local .
```

The inherited Compose examples reference upstream images. Set their `image`
to `birdnet-go-plus:local` before deploying the fork; upstream images do not
contain its features.

For source builds and the Windows development package, see the
[main installation guide](../README.md#install). Install and authentication-reset
scripts are under [`scripts/`](../scripts/).
