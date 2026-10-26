# Official NetworkConfig descriptor

Sources copied without modifications from EasyTier v2.6.4 (`8428a89d`),
`easytier/src/proto/`. Upstream license: LGPLv3
(https://github.com/EasyTier/EasyTier/blob/v2.6.4/LICENSE). A copy of the
license and corresponding-source link is included in `THIRD_PARTY_NOTICES.md`.

`v2.6.4.pb` is a FileDescriptorSet with dependencies, used by protobuf's
`dynamicpb` and `protojson`. This keeps the remote editor independent from
our simplified local Config model. Binary unknown fields and config source
are retained; JSON rejects unknown names. The editor pins instance_id.

Regenerate from the repository root:

```sh
uv run --with grpcio-tools python -m grpc_tools.protoc \
  -I internal/easytier/schema --include_imports \
  --descriptor_set_out=internal/easytier/schema/v2.6.4.pb \
  internal/easytier/schema/api_manage.proto
```

No Python/protoc dependency is needed at application build or runtime.
