# obey-installer

Installer engine and plugin manager for `fest`, `camp`, and future Obey tools.

## Status

Private incubation repo. Not yet ready for external use. The mature engine will
merge into the public `festival` binary once foundations stabilize. Until then,
`projects/festival` remains the public bootstrap surface.

## Design Reference

See `workflow/design/festival-plugin-marketplace/` in the
`obey-campaign` workspace for the architecture and rollout plan.

## Build

```bash
just build       # builds bin/obey-installer
just test        # go test ./...
just lint        # golangci-lint
```

## License

FSL-1.1-ALv2. See LICENSE.
