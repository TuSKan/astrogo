---
type: Added
pr: 195
---
**`api.RetryPolicy` — the extension point `remote.ErrRetriable` had been documenting for
months without it existing.** A per-client `func(api.Attempt) bool`, with
`api.DefaultRetryPolicy` exported so a custom one can defer to it rather than restate the
429 / 5xx-except-501 / no-response rule.
