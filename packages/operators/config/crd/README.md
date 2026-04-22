# CRD manifests

Intentionally empty. Real CRD YAML is generated from the Go types via:

```sh
controller-gen crd paths=../../api/v1alpha1/... output:crd:dir=.
```

The coordinator will wire this into the top-level build tooling in a
later wave.
