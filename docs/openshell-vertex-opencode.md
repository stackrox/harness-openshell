# OpenShell Vertex inference provider configuration

OpenShell's Vertex Claude route exposes the Anthropic Messages API at
`https://inference.local/v1`. OpenCode must use an internal Anthropic-compatible
provider for this route; it must not select OpenCode's public Vertex runtime.

```jsonc
{
  "$schema": "https://opencode.ai/config.json",
  "providers": {
    "openshell-inference": {
      "name": "OpenShell Vertex inference route",
      "package": "@opencode/ai/providers/anthropic",
      "settings": {
        "baseURL": "https://inference.local/v1"
      },
      "models": {
        "claude-haiku-4-5@20251001": {
          "name": "Claude Haiku 4.5",
          "modelID": "claude-haiku-4-5-20251001"
        },
        "claude-sonnet-4-5@20250929": {
          "name": "Claude Sonnet 4.5",
          "modelID": "claude-sonnet-4-5-20250929"
        },
        "claude-opus-4-5@20251101": {
          "name": "Claude Opus 4.5",
          "modelID": "claude-opus-4-5-20251101"
        }
      }
    }
  },
  "model": "openshell-inference/claude-haiku-4-5@20251001"
}
```

The provider keys are internal OpenCode names. The `modelID` values are the
Anthropic Messages API identifiers: Vertex's `@` version suffix is removed or
translated to the dated Anthropic identifier.

When running OpenCode directly in an image, place the configuration at the
global OpenCode location and provide only a placeholder key:

```dockerfile
ENV HOME=/tmp/opencode-home \
    XDG_CONFIG_HOME=/tmp/opencode-home/.config \
    XDG_DATA_HOME=/tmp/opencode-home/.local/share \
    XDG_CACHE_HOME=/tmp/opencode-home/.cache \
    ANTHROPIC_API_KEY=unused

RUN mkdir -p /tmp/opencode-home/.config/opencode \
    && chown -R 1001:0 /tmp/opencode-home

COPY --chown=1001:0 config/opencode.jsonc \
  /tmp/opencode-home/.config/opencode/opencode.jsonc
```

The placeholder is for the local compatible client. OpenShell supplies the
gateway-managed Vertex credentials; the sandbox must not have direct access to
Google metadata, Vertex, or public Anthropic endpoints.

For an embedded OpenCode SDK, map public model references to the internal
provider before creating a session:

```ts
const runtimeModel = {
  providerID: "openshell-inference",
  modelID: "claude-haiku-4-5@20251001",
};
```

Do not pass `google-vertex-anthropic/...` in OpenShell mode. The gateway must
have a matching `google-vertex-ai` inference route, and policy should allow
only `inference.local:443` plus `models.opencode.ai:443` for the OpenCode
runtime.
