# nix2container

> **This is a fork of [nlewo/nix2container](https://github.com/nlewo/nix2container).** Its `next` branch is upstream `master` with the pull requests below merged. All of them are open against the original repository, and the intention is that they are merged there. This fork is not maintained as a separate project: it carries nothing that is not a pull request upstream, `master` tracks upstream `master`, and once the pull requests land upstream the fork has no reason to exist.
>
> What the merged pull requests change, and why:
>
> - [#207](https://github.com/nlewo/nix2container/pull/207): the layers taken from `fromImage` record their size. Without it, a push had to read the whole blob to learn a number the manifest already carries.
> - [#209](https://github.com/nlewo/nix2container/pull/209): an image with no layers serialises `"layers": []` instead of `null`, so every reader of the image JSON can iterate the field without a special case.
> - [#219](https://github.com/nlewo/nix2container/pull/219): with `reproducible = false`, each layer tar written to the store held the paths of every layer, not its own. The digests were right; the files were not.
> - [#220](https://github.com/nlewo/nix2container/pull/220): `buildLayer { layersFile }` takes a layer split computed by another tool, so the grouping of the closure is an input rather than something nix2container has to decide well for every image. The `store_layers` of nixpkgs' `streamLayeredImage` is one such input, which gives the same layers as `dockerTools` for the same closure.
> - [#221](https://github.com/nlewo/nix2container/pull/221): `go.podman.io/image` was imported for one struct, and pulled about a hundred modules with it. A local type reads the one field that was used, and the module graph shrinks from 140 to 42 modules.
> - [#211](https://github.com/nlewo/nix2container/pull/211): `includeStorePaths = false` ships the listed paths without their runtime closure, for containers whose `/nix/store` is provided at run time. Baking the closure into layers there only duplicates what the mount already has.
> - [#224](https://github.com/nlewo/nix2container/pull/224): building a layer tar allocated a buffer per file and opened every directory; the blob file was written in 512-byte pieces. One pooled buffer and buffered writes make the tar step about a third faster on trees with many small files, with the same bytes.
> - [#208](https://github.com/nlewo/nix2container/pull/208): `fromImageEnv = true` keeps the `Env` of the base image that the image config does not set. A base that sets `PATH` or CUDA variables no longer has to be repeated by hand. Opt-in, so existing images do not change.
> - [#210](https://github.com/nlewo/nix2container/pull/210): `perms.orMode` adds permission bits without replacing the mode. A store tree mixes `0444` and `0555` files, and "make it writable" with `mode` alone either drops or grants the execute bit on all of them.
> - [#222](https://github.com/nlewo/nix2container/pull/222): `compressor = "gzip"` compresses each layer once, at build time, with deterministic output. The compressed digest is then known before the push, so a repush only asks the registry which blobs it lacks, and every builder produces the same bytes. The cost is store space: the output holds the compressed layers.
> - [#223](https://github.com/nlewo/nix2container/pull/223): `compressor = "zstd"`, for OCI destinations: faster to produce and smaller than gzip.
> - [#225](https://github.com/nlewo/nix2container/pull/225): gzip layers are compressed with `klauspost/compress`, the dependency #223 already brings, at more than twice the speed of the standard library for the same level. The digests change once.
> - [#226](https://github.com/nlewo/nix2container/pull/226): `buildLayer { permsFile }` passes a perms list produced by a build, for permissions that live in a store path rather than being known at eval time. `--perms` already took a file.
> - [#228](https://github.com/nlewo/nix2container/pull/228): a `perms` regex was compiled for every file it was checked against. The two shapes generated perms lists use, an exact path and a subtree, are now matched by string comparison; a path of 42 000 files with 41 entries goes from over half a minute to under three seconds, same digest.
> - [#230](https://github.com/nlewo/nix2container/pull/230): two refactors with no behaviour change: each leaf of the layer graph carries a *source* it is read through, and the options that shape a layer are one `LayerOptions` struct, so the next three features are one field each rather than one more positional argument on `NewLayers`.
> - [#231](https://github.com/nlewo/nix2container/pull/231): `buildLayer { fromTar }` takes a tar archive as the content of a store path, with the ownership and modes of the archive headers. A customisation layer built under fakeroot has those only in the tar it produces; unpacking it into the store throws them away.
> - [#232](https://github.com/nlewo/nix2container/pull/232): `buildLayer { ensureDirs }` carries directories at a fixed owner and mode, and creates them when no source path has them: the `/nix` and `/nix/store` above a shipped store, which are the parents of the paths, not their content.
> - [#227](https://github.com/nlewo/nix2container/pull/227): `buildLayer { excludes }` leaves subtrees of a store path out of the layer at emission time. Leaving part of a path out used to mean a pruned copy of it, which is a new store path with its own closure.
> - [#229](https://github.com/nlewo/nix2container/pull/229): the layers of an image are compressed in parallel, at most `GOMAXPROCS` at a time, with the bytes of each layer unchanged; three large layers take the time of the largest one alone.
>
> Two things exist only in this merge, and go upstream with whichever of the pull requests concerned lands second: `--layers-json` and `--compressor` together (`NewLayersCompressedFromSplit`; #220 and #222 were written independently), and the `Env` field that #208 reads on the type #221 introduced.

## Getting started

```nix
{
  inputs.nix2container.url = "github:nlewo/nix2container";

  outputs = { self, nixpkgs, nix2container }: let
    pkgs = import nixpkgs { system = "x86_64-linux"; };
    nix2containerPkgs = nix2container.packages.x86_64-linux;
  in {
    packages.x86_64-linux.hello = nix2containerPkgs.nix2container.buildImage {
      name = "hello";
      config = {
        entrypoint = ["${pkgs.hello}/bin/hello"];
      };
    };
  };
}
```

This image can then be loaded into Docker with

```
$ nix run .#hello.copyToDockerDaemon
$ docker run hello:latest
Hello, world!
```


## More Examples

To load and run the `bash` example image into Podman:

```
$ nix run github:nlewo/nix2container#examples.bash.copyToPodman
$ podman run -it bash
```

- [`bash`](./examples/bash.nix): Bash in `/bin/`
- [`fromImage`](./examples/from-image.nix): Alpine as base image
- [`fromImageManifest`](./examples/from-image-manifest.nix): Alpine as base image, from a stored `manifest.json`.
- [`nginx`](./examples/nginx.nix)
- [`nonReproducible`](./examples/non-reproducible.nix): with a non reproducible store path :/
- [`openbar`](./examples/openbar.nix): set permissions on files (without root nor VM)
- [`uwsgi`](./examples/uwsgi/default.nix): isolate dependencies in layers
- [`layered`](./examples/layered.nix): build a layered image as described in [this blog post](https://grahamc.com/blog/nix-and-layered-docker-images)


## Functions documentation

### `nix2container.buildImage`

Function arguments are:

- **`name`** (required): the name of the image.

- **`tag`** (defaults to the image output hash): the tag of the image.

- **`config`** (defaults to `{}`): an attribute set describing an image configuration as
    defined in the [OCI image
    specification](https://github.com/opencontainers/image-spec/blob/8b9d41f48198a7d6d0a5c1a12dc2d1f7f47fc97f/specs-go/v1/config.go#L23).

- **`copyToRoot`** (defaults to `null`): a derivation (or list of
    derivations) copied in the image root directory (store path
    prefixes `/nix/store/hash-path` are removed, in order to relocate
    them at the image `/`).

    `pkgs.buildEnv` can be used to build a derivation which has to be copied to
    the image root. For instance, to get bash and coreutils in the image `/bin`:
    ```
    copyToRoot = pkgs.buildEnv {
      name = "root";
      paths = [ pkgs.bashInteractive pkgs.coreutils ];
      pathsToLink = [ "/bin" ];
    };
    ```

- **`fromImage`** (defaults to `null`): an image that is used as base
    image of this image; use `pullImage` or `pullImageFromManifest` to
    supply this.

- **`fromImageEnv`** (defaults to `false`): keep the `Env` entries of
    `fromImage`. An entry is dropped when `config` sets the same
    variable, and the entries of `config` come last. The other fields
    of the base configuration are not inherited.

- **`includeStorePaths`** (defaults to `true`): see
    `buildLayer.includeStorePaths`. It applies to the image layers and
    not to layers added with the `buildImage.layers` attribute.

- **`maxLayers`** (defaults to `1`): the maximum number of layers to
    create. This is based on the store path "popularity" as described
    in this [blog
    post](https://grahamc.com/blog/nix-and-layered-docker-images). Note
    this is applied on the image layers and not on layers added with
    the `buildImage.layers` attribute.

- **`compressor`** (defaults to `null`): see `buildLayer.compressor`.
    It applies to the image layers and not to layers added with the
    `buildImage.layers` attribute.

- **`perms`** (defaults to `[]`): a list of file permisssions which are
    set when the tar layer is created: these permissions are not
    written to the Nix store.

    Each element of this permission list is a dict such as
    ```
    { path = "a store path";
      regex = ".*";
      mode = "0664";
    }
    ```
    The mode is applied on a specific path. In this path subtree,
    the mode is then applied on all files matching the regex.

    `mode` sets the mode, and `orMode` adds bits to it. For instance,
    `orMode = "0200";` makes the files writable by their owner and
    keeps their execute bits. With both, `mode` comes first:
    `{ mode = "0444"; orMode = "0200"; }` gives `0644`. The entries
    are applied in list order.

- **`initializeNixDatabase`** (defaults to `false`): to initialize the
    Nix database with all store paths added into the image. Note this
    is only useful to run nix commands from the image, for instance to
    build an image used by a CI to run Nix builds.

- **`layers`** (defaults to `[]`): a list of layers built with the
    buildLayer function: if a store path in deps or contents belongs
    to one of these layers, this store path is skipped. This is pretty
    useful to isolate store paths that are often updated from more
    stable store paths, to speed up build and push time.


### `nix2container.pullImage`

Pull an image from a container registry by name and tag/digest, storing the
entirety of the image (manifest and layer tarballs) in a single store path.
The supplied `sha256` is the narhash of that store path.

Function arguments are:

- **`imageName`** (required): the name of the image to pull.

- **`imageDigest`** (required): the digest of the image to pull.

- **`sha256`** (required): the sha256 of the resulting fixed output derivation.

- **`os`** (defaults to `linux`)

- **`arch`** (defaults to `x86_64`)

- **`tlsVerify`** (defaults to `true`)


### `nix2container.pullImageFromManifest`

Pull a base image from a container registry using a supplied manifest file, and the
hashes contained within it. The advantages of this over the basic `pullImage`:

- Each layer archive is in its own store path, which means each will download just once
  and naturally deduplicate for multiple base images that share layers.
- There is no Nix-specific hash, so it's possible update the base image by simply
  re-fetching the `manifest.json` from the registry; no need to actually pull the whole
  image just to compute a new narhash for it.

With this function the `manifest.json` acts as a lockfile meant to be stored in
source control alongside the Nix container definitions. As a convenience, the manifest
can be fetched/updated using the supplied passthru script, eg:

```
nix run .#examples.fromImageManifest.fromImage.getManifest > examples/alpine-manifest.json
```

Function arguments are:

- **`imageName`** (required): the name of the image to pull.

- **`imageManifest`** (required): the manifest file of the image to pull.

- **`imageTag`** (defaults to `latest`)

- **`os`** (defaults to `linux`)

- **`arch`** (defaults to `x86_64`)

- **`tlsVerify`** (defaults to `true`)

- **`registryUrl`** (defaults to `registry.hub.docker.com`)

Note that `imageTag`, `os`, and `arch` do not affect the pulled image; that is
governed entirely by the supplied `manifest.json` file. These arguments are
used for the manifest-selection logic in the included `getManifest` script.


#### Authentication

If the Nix daemon is used for building, here is how to set up registry
authentication.

1. `docker login URL` to whatever it is
2. Copy `~/.docker/config.json` to `/etc/nix/skopeo/auth.json`
3. Make the directory and all the files readable to the `nixbld` group:
   ```
   sudo chmod -R g+rx /etc/nix/skopeo
   sudo chgrp -R nixbld /etc/nix/skopeo
   ```
4. Bind mount the file into the Nix build sandbox
   ```
   extra-sandbox-paths = /etc/skopeo/auth.json=/etc/nix/skopeo/auth.json
   ```

Every time a new registry authentication has to be added, update
`/etc/nix/skopeo/auth.json` file.


### `nix2container.buildLayer`

For most use cases, this function is not required. However, it could be
useful to explicitly isolate some parts of the image in dedicated
layers, for caching (see the "Isolate dependencies in dedicated
layers" section) or non reproducibility (see the `reproducible`
argument) purposes.

Function arguments are:

- **`deps`** (defaults to `[]`): a list of store paths to include in the
    layer.

- **`copyToRoot`** (defaults to `null`): a derivation (or list of
    derivations) copied in the image root directory (store path
    prefixes `/nix/store/hash-path` are removed, in order to relocate
    them at the image `/`).

    `pkgs.buildEnv` can be used to build a derivation which has to be copied to
    the image root. For instance, to get bash and coreutils in the image `/bin`:
    ```
    copyToRoot = pkgs.buildEnv {
      name = "root";
      paths = [ pkgs.bashInteractive pkgs.coreutils ];
      pathsToLink = [ "/bin" ];
    };
    ```

- **`reproducible`** (defaults to `true`): If `false`, the layer tarball
    is stored in the store path. This is useful when the layer
    dependencies are not bit reproducible: it allows to have the layer
    tarball and its hash in the same store path.

- **`includeStorePaths`** (defaults to `true`): when `false`, the
    layer holds the paths listed in `deps` and `copyToRoot`, but not
    their runtime closure. The listed paths keep their store paths, so
    their references must be present at run time, for instance through
    a `/nix/store` mounted into the container. Those references are not
    pushed with the image. dockerTools' `streamLayeredImage` has an
    option with the same name, but it does not ship the store paths.

- **`maxLayers`** (defaults to `1`): the maximum number of layers to
    create. This is based on the store path "popularity" as described
    in this [blog
    post](https://grahamc.com/blog/nix-and-layered-docker-images). Note
    this is applied on the image layers and not on layers added with
    the `buildLayer.layers` attribute.

- **`layersFile`** (defaults to `null`): a JSON file with the layer
    split to use instead of `maxLayers`: a list of store path lists,
    one list per layer, in order. Every path of the layer closure (the
    closure of `deps` and `copyToRoot`, without `ignore`) must appear
    in exactly one list, and no other path may appear. This lets the
    split come from another tool, for instance the `store_layers` of
    the `conf.json` that nixpkgs' `streamLayeredImage` writes.
- **`compressor`** (defaults to `null`): set it to `"gzip"` or
    `"zstd"` to compress the layers at build time. The compressed
    blobs are stored in the layer derivation output, and they are
    pushed as they are, so a push does not tar the store paths again.
    The compression is deterministic (gzip: level 6, no timestamp, no
    file name, OS set to 255; zstd: level 3, one encoder goroutine),
    so the same layer always has the same digest. Only OCI
    destinations accept zstd layers: use `"gzip"` for `docker-daemon`
    and for registries that only know the Docker schema 2 media types.
    The cost is store space: the output holds the compressed layers,
    not only their JSON description. It requires
    `reproducible = true`.

- **`perms`** (defaults to `[]`): a list of file permisssions which are
    set when the tar layer is created: these permissions are not
    written to the Nix store.

- **`fromTar`** (defaults to `[]`): a list of `{ path = <store path>;
    tar = <tar archive>; }`. The members of the archive are the content
    of the store path, with the ownership and modes of the archive
    headers. This is for a layer built under fakeroot, whose owners and
    modes exist only in the tar it produces. Entry names are taken
    relative to the archive root. Hard links, devices and fifos are
    refused.

- **`ensureDirs`** (defaults to `[]`): a list of `{ path = <store
    path>; dir = "relative/dir"; uid; gid; mode; }`. The directory is
    carried at that owner and mode, and created when the source lacks
    it, for instance `/nix` and `/nix/store` above a shipped store, which
    no store path contains.
- **`excludes`** (defaults to `[]`): subtrees of a store path left out
    of the layer, as `{ path = <store path>; excludes = [ "share/doc"
    ... ]; }` with paths relative to the store path. The store path is
    still added with the rest of its content. This avoids a pruned copy
    of the path, which would be a new store path with a new closure.
- **`permsFile`** (defaults to `null`): a JSON file holding the list
    `perms` would hold, for permissions computed by a build rather than
    known at eval time. Exactly one of `perms` and `permsFile`.

    Each element of this permission list is a dict such as
    ```
    { path = "a store path";
      regex = ".*";
      mode = "0664";
    }
    ```
    The mode is applied on a specific path. In this path subtree,
    the mode is then applied on all files matching the regex.

    `mode` sets the mode, and `orMode` adds bits to it. For instance,
    `orMode = "0200";` makes the files writable by their owner and
    keeps their execute bits. With both, `mode` comes first:
    `{ mode = "0444"; orMode = "0200"; }` gives `0644`. The entries
    are applied in list order.

- **`layers`** (defaults to `[]`): a list of layers built with the
    `buildLayer` function: if a store path in deps or contents belongs
    to one of these layers, this store path is skipped. This is pretty
    useful to isolate store paths that are often updated from more
    stable store paths, to speed up build and push time.

- **`ignore`** (defaults to `null`): a store path to ignore when
    building the layer. This is mainly useful to ignore the
    configuration file from the container layer.

- **`metadata`** (defaults to `{ created_by = "nix2container"; }`): an attribute
    set containing this layer's `created_by`, `author` and `comment` values

## Isolate dependencies in dedicated layers

It is possible to isolate application dependencies in a dedicated
layer. This layer is built by its own derivation: if storepaths
composing this layer don't change, the layer is not rebuilt. Moreover,
Skopeo can avoid to push this layer if it has already been pushed.

Let's consider an `application` printing a conversation. This script
depends on `bash` and the `hello` binary. Because most of the changes
concern the script itself, it would be nice to isolate scripts
dependencies in a dedicated layer: when we modify the script, we only
need to rebuild and push the layer containing the script. The layer
containing dependencies won't be rebuilt and pushed.

As shown below, the `buildImage.layers` attribute allows to
explicitly specify a set of dependencies to isolate.

```nix
{ pkgs }:
let
  application = pkgs.writeScript "conversation" ''
    ${pkgs.hello}/bin/hello
    echo "Haaa aa... I'm dying!!!"
  '';
in
pkgs.nix2container.buildImage {
  name = "hello";
  config = {
    entrypoint = ["${pkgs.bash}/bin/bash" application];
  };
  layers = [
    (pkgs.nix2container.buildLayer { deps = [pkgs.bash pkgs.hello]; })
  ];
}
```

This image contains 2 layers: a layer with `bash` and `hello` closures
and a second layer containing the script only.

In real life, the isolated layer can contains a Python environment or
Node modules.

See
[Nix & Docker: Layer explicitly without duplicate packages!](https://blog.eigenvalue.net/2023-nix2container-everything-once/)
for learning how to avoid duplicate store paths in your explicitly layered
images.

## Quick and dirty benchmarks

The main goal of nix2container is to provide fast rebuild/push
container cycles. In the following, we provide an order of magnitude
of rebuild and repush time, for the [`uwsgi` image](https://github.com/nlewo/nix2container/blob/c6a8d82f1cdd80fabb76e7c1459471e1ea95a080/examples/uwsgi/default.nix).

**warning: this is quick and dirty benchmarks which only provide an order of magnitude**

We build the container and push the container. We then made a small
change in the `hello.py` file to trigger a rebuild and a push.

Method | Rebuild/repush time | Executed command
---|---|---
nix2container.buildImage | ~1.8s | `nix run .#example.uwsgi.copyToRegistry`
dockerTools.streamLayeredImage | ~7.5s | `nix build .#example.uwsgi \| docker load`
dockerTools.buildImage | ~10s | `nix build .#example.uwsgi; skopeo copy docker-archive://./result docker://localhost:5000/uwsgi:latest`

Note we could not compare the same distribution mechanisms because
- Skopeo is not able to skip already loaded layers by the Docker daemon and
- Skopeo failed to push to the registry an image streamed to stdin.


## Run the tests

```
nix run .#tests.all
```

This builds several example images with Nix, loads them with Skopeo,
runs them with Podman, and test output logs.

Not that, unfortunately, these tests are not executed in the Nix
sandbox because it is currently not possible to run a container in the
Nix sandbox.

It is also possible to run a specific test:

```
nix run .#tests.basic
```


## The nix2container Go library

This library is currently used by the Skopeo `nix` transport available
in [this branch](https://github.com/nlewo/image/tree/nix).

For more information, refer to [the Go
documentation](https://pkg.go.dev/github.com/nlewo/nix2container).


## Commercial support

For commercial support (customizations, image optimizations and best
practices guidance, bug fixes), please contact
[nlewo](https://github.com/nlewo) at antoine@lewocorp.eu.
