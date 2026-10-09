# ai-proxy

Helm chart for jo-hoe/ai-proxy — OIDC-auth reverse proxy for LLM APIs.

![Version: 0.9.0](https://img.shields.io/badge/Version-0.9.0-informational?style=flat-square) 
![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) 
![AppVersion: 0.9.0](https://img.shields.io/badge/AppVersion-0.9.0-informational?style=flat-square) 

## Overview

`ai-proxy` is an OIDC-authenticating reverse proxy for LLM APIs. It exchanges a
long-lived OIDC refresh token for short-lived access tokens and injects them
into every upstream request, so downstream clients only need to know a static
proxy URL — no per-user auth wiring.

The chart supports two modes for supplying the OIDC token:

1. **Mounted Secret** (recommended). Set `oidc.endpoint`, `oidc.clientId`, and
   `oidc.refreshToken` — or point `oidc.existingSecret` at an existing Secret
   with keys `oidc-endpoint`, `oidc-client-id`, `refresh-token`. The proxy
   auto-activates on startup, so image updates require no manual intervention.
2. **Secretless / manual push**. Deploy with no token, then `POST /token` to the
   management API. Useful when the token isn't available at deploy time.

With `oidc.persistSecret` enabled (**the default**), the proxy writes the pushed
token and every rotated refresh token back into the chart-rendered Secret via the
k8s API. This makes the secretless mode **restart-durable**: after a pod restart
or chart upgrade the proxy re-reads the Secret and re-activates automatically —
no fresh `POST /token` needed. It requires `rbac.enabled: true` (also the default)
so the chart can grant a tightly-scoped Role. The Role is limited to `get`/`patch`
on the single token Secret; `create` is deliberately not granted because it cannot
be restricted by `resourceNames`, so the chart (not the proxy) owns the Secret.
Set `oidc.persistSecret: false` to opt out — pushed tokens then live only in
memory and are lost on restart.

> **Readiness:** the pod's readiness probe is `/healthz`, which reports Ready only
> once a token is loaded. A pure-secretless pod stays NotReady (and the proxy
> Service won't route to it) until the first `POST /token`. The management port
> 7656 remains reachable while NotReady, so the token push still works.

## Endpoints

| Port | Purpose |
|------|---------|
| 7655 | LLM reverse proxy — downstream clients call this |
| 7656 | Management API — `POST /token`, `GET /status`, `GET /healthz` |

## Installation

```bash
helm install ai-proxy oci://ghcr.io/jo-hoe/charts/ai-proxy \
  --set config.upstreamUrl=https://api.anthropic.com \
  --set oidc.endpoint=https://your-idp.example.com/oauth2/token \
  --set oidc.clientId=your-client-id \
  --set oidc.refreshToken=your-refresh-token
```

## Values
| Key | Type | Default | Description |
|-----|------|---------|-------------|
| affinity | object | `{}` | Node/pod affinity rules. |
| config.clientVersion | string | `"1.4.7"` | Client version reported to the upstream API. The upstream rejects callers below its minimum with HTTP 426; bump this when that minimum rises. Must be valid semver. |
| config.logLevel | string | `"INFO"` | Log level for the proxy. One of: DEBUG, INFO, WARN, ERROR. |
| config.rotationMarginSeconds | int | `600` | How many seconds before token expiry to trigger a proactive rotation. Increase if your OIDC provider is slow to respond. |
| config.upstreamUrl | string | `""` | Required. Base URL of the upstream LLM API to proxy to. |
| fullnameOverride | string | `""` | Fully override the generated resource names. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. `IfNotPresent` | `Always` | `Never`. |
| image.repository | string | `"ghcr.io/jo-hoe/ai-proxy"` | Container image repository. |
| image.tag | string | `""` | Container image tag. Defaults to the chart's appVersion when empty. |
| ingress.annotations | object | `{}` | Extra annotations for both ingress resources. |
| ingress.className | string | `""` | Ingress class name. Leave empty to omit the field. |
| ingress.enabled | bool | `false` | Enable Ingress resources for proxy and management endpoints. |
| ingress.mgmtHost | string | `"ai-proxy-mgmt.example.com"` | Hostname for the management API ingress. |
| ingress.mgmtTlsSecretName | string | `""` | Name of the management TLS Secret. Defaults to `<fullname>-mgmt-tls` when empty. |
| ingress.proxyHost | string | `"ai-proxy.example.com"` | Hostname for the LLM proxy ingress. |
| ingress.tls | bool | `false` | Opt-in TLS. When true, references the Secret named in `tlsSecretName`. You must provision the Secret separately (cert-manager, external-secrets, manual). |
| ingress.tlsSecretName | string | `""` | Name of the TLS Secret. Defaults to `<fullname>-tls` when empty. |
| nameOverride | string | `""` | Override the chart name portion of resource names. |
| nodeSelector | object | `{}` | Node labels for pod placement. |
| oidc.clientId | string | `""` | OAuth client ID. Only used when `existingSecret` is empty. |
| oidc.endpoint | string | `""` | OIDC token endpoint URL. Only used when `existingSecret` is empty. |
| oidc.existingSecret | string | `""` | Reference a pre-existing Secret with keys `oidc-endpoint`, `oidc-client-id`, `refresh-token`. Takes precedence over the inline values. |
| oidc.persistSecret | bool | `true` | When true (default), the proxy patches the mounted Secret with the latest credentials after every token push and rotation, so the pod re-activates after a restart or chart upgrade without a fresh POST /token. This makes a secretless deploy restart-durable. Requires `rbac.enabled: true`. Set to false to opt out (pushed tokens then live only in memory and are lost on restart). |
| oidc.refreshToken | string | `""` | Refresh token. Only used when `existingSecret` is empty. For dev/testing only — prefer `existingSecret` in production. |
| rbac.enabled | bool | `true` | Create ServiceAccount + Role + RoleBinding. Set to false if your cluster provisions these externally. |
| replicaCount | int | `1` | Number of proxy replicas. |
| resources | object | `{}` | Pod resource requests and limits. |
| service.mgmtPort | int | `7656` | Port 7656 — management API (POST /token, GET /status). |
| service.proxyPort | int | `7655` | Port 7655 — the LLM reverse proxy (consumers call this). |
| service.type | string | `"ClusterIP"` | Service type. `ClusterIP` | `LoadBalancer` | `NodePort`. |
| serviceAccount.name | string | `""` | Name of the ServiceAccount to use or create. Defaults to the fullname when empty. |
| tolerations | list | `[]` | Tolerations for pod scheduling on tainted nodes. |

## Source Code

* <https://github.com/jo-hoe/ai-proxy>


----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
