# Claude Code with LiteLLM

[Back to the README](../README.md#claude-code-with-litellm)

The declarative LiteLLM deployment lives in [`configs/litellm/`](../configs/litellm/). It exposes GitHub Copilot and W&B Inference through one local Anthropic-compatible gateway. The Compose service is intentionally stateless except for the persistent `litellm-copilot-data` volume that stores GitHub's OAuth credential.

## Usage

```bash
litellm-proxy.sh start
litellm-proxy.sh status
litellm-proxy.sh models
litellm-proxy.sh test-copilot
litellm-proxy.sh test-wandb
litellm-proxy.sh claude
litellm-proxy.sh claude --dangerously-skip-permissions --allow-dangerously-skip-permissions
```

On the first `litellm-proxy.sh start`, follow the GitHub device-login URL and code printed by the script. The default Claude Code model is `claude-fable-5-1`; set `LITELLM_MODEL` to another model returned by `litellm-proxy.sh models`, such as `claude-sonnet-4-6` or `wandb/zai-org/GLM-5.2`. Every argument after the `claude` subcommand is passed directly to Claude Code.

## Credentials

Each secret is read from its environment variable first, then from the macOS Keychain entry whose service name equals the variable name: `LITELLM_MASTER_KEY` (proxy key), `LITELLM_SALT_KEY` (encrypts provider keys stored in the proxy DB; set it once and never change it), `WANDB_API_KEY` (W&B Inference), `WANDB_QA_API_KEY` (`wandb-qa/` models on `api.qa.inference.wandb.ai`), `WANDB_OPENAI_PROJECT` (production project), and `WANDB_QA_OPENAI_PROJECT` (QA project). Only `LITELLM_MASTER_KEY` and `LITELLM_SALT_KEY` are required to start the proxy. The W&B settings are optional at startup; requests to W&B models need the corresponding API key and project value. The project values supply the upstream `OpenAI-Project` header and stay outside the repository. To add or rotate a Keychain value:

```bash
security add-generic-password -U -a "$USER" -s LITELLM_MASTER_KEY -w '<LiteLLM proxy key>'
security add-generic-password -U -a "$USER" -s LITELLM_SALT_KEY -w '<LiteLLM salt key>'
security add-generic-password -U -a "$USER" -s WANDB_API_KEY -w '<W&B API key>'
security add-generic-password -U -a "$USER" -s WANDB_QA_API_KEY -w '<W&B QA API key>'
security add-generic-password -U -a "$USER" -s WANDB_OPENAI_PROJECT -w '<W&B entity/project>'
security add-generic-password -U -a "$USER" -s WANDB_QA_OPENAI_PROJECT -w '<W&B QA entity/project>'
```
