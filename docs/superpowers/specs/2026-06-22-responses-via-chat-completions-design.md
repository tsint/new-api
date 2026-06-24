# Responses API via Chat Completions compatibility design

## Summary

new-api already accepts `POST /v1/responses` and can relay it to an upstream
Responses endpoint. It also contains the inverse compatibility path (a Chat
Completions client routed to a Responses-only upstream). This change adds the
missing direction: a Responses client can be routed to a channel whose upstream
only implements `POST /v1/chat/completions`.

The bridge is opt-in and selected after channel routing. It translates the
request to Chat Completions, invokes the existing channel adaptor, and translates
the upstream JSON or SSE response back to the Responses protocol. Native
Responses relay remains the default.

## Goals

- Support the useful, portable subset of `POST /v1/responses` on existing Chat
  Completions channels.
- Preserve explicit zero and `false` request values.
- Produce Responses-shaped non-streaming responses and semantic Responses SSE
  events rather than exposing Chat Completions objects.
- Support text, image input, structured output, and custom function calling.
- Reuse channel model mapping, parameter overrides, retries, billing, usage
  accounting, stream timeouts, and error handling.
- Fail explicitly when a Responses feature cannot be represented by a single
  Chat Completions call.

## Non-goals

- Emulating OpenAI server-side response storage, Conversations, or
  `previous_response_id` state.
- Emulating Responses built-in tools (web/file search, computer use, code
  interpreter, image generation, shell, or remote MCP) inside the gateway.
- Supporting background Responses jobs or `/v1/responses/compact` through Chat
  Completions.
- Guaranteeing reasoning-item or encrypted-reasoning portability.
- Synthesizing citations, annotations, refusal metadata, logprobs, or tool output
  that the Chat Completions upstream did not return.

## Why the bridge is necessarily partial

OpenAI describes Responses as an item-based, stateful, agentic API, while Chat
Completions is message-based. A message, function call, and function call output
are separate Responses items. Responses also provides server-managed state and
built-in tools that have no Chat Completions equivalent. The bridge therefore
uses three compatibility classes:

1. **Lossless:** direct structural mapping, including text messages, sampling
   parameters, custom functions, and token usage.
2. **Defined degradation:** a Chat completion choice becomes one Responses
   output sequence; absent Chat metadata is represented by empty/default
   Responses fields.
3. **Rejected:** stateful or built-in-tool behavior that cannot be reproduced
   honestly by one upstream request.

## Activation and routing

Add `global.responses_to_chat_completions_policy` with the same selector shape
as the existing `global.chat_completions_to_responses_policy`:

```json
{
  "enabled": true,
  "all_channels": false,
  "channel_ids": [12],
  "channel_types": [1],
  "model_patterns": ["^gpt-4o", "^deepseek-"]
}
```

The bridge is used only when all conditions hold:

- relay mode is `RelayModeResponses` (never compaction);
- global and channel pass-through-body modes are disabled;
- the selected channel and original client model match the policy.

Selection occurs inside each retry attempt, after `InitChannelMeta`, so a retry
may choose a native Responses channel or a Chat-only channel independently.
The request conversion chain records
`openai_responses -> openai`. The public relay format remains Responses for
error and response serialization.

## Request mapping

### Top-level fields

| Responses request | Chat Completions request | Rule |
|---|---|---|
| `model` | `model` | Model mapping still runs before conversion. |
| `input: string` | one `user` message | Empty string remains present. |
| `input: InputItem[]` | `messages` | See item mapping below. |
| `instructions` | leading `developer` message | String only; preserve separately from input. |
| `max_output_tokens` | `max_completion_tokens` | Preserve explicit zero via pointer fields. |
| `temperature` | `temperature` | Preserve explicit `0`. |
| `top_p` | `top_p` | Preserve explicit `0`. |
| `stream` | `stream` | Preserve explicit `false`. |
| `stream_options.include_obfuscation` | none | Ignored only when false; true is rejected. |
| `parallel_tool_calls` | `parallel_tool_calls` | Must decode as boolean. |
| `tools` custom function | `tools[].function` | Flattened Responses function schema becomes nested Chat schema. |
| `tool_choice` | `tool_choice` | Named function choice is nested under `function`. |
| `text.format` | `response_format` | `json_schema`, `json_object`, and `text` are supported. |
| `reasoning.effort` | `reasoning_effort` | Summary/encrypted-content options are not synthesized. |
| `store` | `store` | Forwarded; it does not provide Responses state semantics. |
| `metadata` | `metadata` | Forwarded. |
| `user` | `user` | Forwarded. |
| `service_tier` | `service_tier` | Existing field filters still apply. |
| `prompt_cache_key` | `prompt_cache_key` | Forwarded when representable as string. |
| `prompt_cache_retention` | same | Forwarded. |
| `safety_identifier` | same | Existing field filters still apply. |
| `top_logprobs` | `top_logprobs` + `logprobs=true` | Preserve explicit zero. |
| `truncation: disabled` or omitted | none | Chat default behavior. |

### Input item mapping

- A message item maps to one Chat message with the same role.
- `input_text` and `output_text` parts map to Chat `text` parts.
- `input_image` maps to `image_url`, retaining URL/data URI and `detail`.
- Unsupported content types (file IDs/URLs, input audio, refusal parts) return a
  400 compatibility error until an explicit mapping exists.
- A `function_call` item maps to an assistant message with `tool_calls`.
  Consecutive function-call items are grouped into one assistant message.
- A `function_call_output` maps to a `tool` message whose `tool_call_id` is the
  Responses `call_id` and whose content is the output string. Missing call IDs
  are rejected.
- Empty input arrays are rejected by normal request validation; empty textual
  content inside a valid item is preserved.

### Unsupported request features

Return HTTP 400 with code `invalid_request_error` (and skip retry) for:

- non-empty `previous_response_id` or `conversation`;
- any non-function tool type;
- `prompt` template references;
- `max_tool_calls` (cannot enforce reliably around a single Chat call);
- non-disabled `truncation`;
- `stream_options.include_obfuscation=true`;
- malformed raw JSON, unknown input items/content types, or an invalid named
  tool choice.

`include` may contain only fields the bridge already emits. Unsupported include
requests are rejected rather than silently returning incomplete data.

## Non-streaming response mapping

Exactly one Chat choice is accepted. A response with no choices or more than one
choice is an upstream protocol error.

The returned object has:

- `id`: upstream Chat ID when it already starts with `resp_`; otherwise a stable
  request-local `resp_` ID derived/generated for this response;
- `object: "response"`;
- `created_at`, `model`, and usage copied from Chat fields;
- `status: "completed"`, except `finish_reason=length` maps to `incomplete` with
  `incomplete_details.reason="max_output_tokens"`;
- one `message` output item containing `output_text` when assistant content is
  non-empty;
- one `function_call` output item per assistant tool call, preserving tool-call
  order, call ID, function name, and complete JSON argument string;
- both message and function-call items when the upstream returns both;
- token usage renamed to `input_tokens`, `output_tokens`, `total_tokens`, with
  cached, reasoning, image, and audio token details copied where available.

Chat `content_filter` maps to an incomplete response. Unknown non-success finish
reasons are represented as incomplete rather than falsely marked completed.

## Streaming response state machine

The bridge consumes Chat Completions SSE and emits Responses SSE objects with a
monotonically increasing `sequence_number`:

1. On the first valid Chat chunk, emit `response.created` and
   `response.in_progress`.
2. On first text content, emit `response.output_item.added` for an assistant
   message and `response.content_part.added` for an empty `output_text` part.
3. For every text delta, emit `response.output_text.delta` with stable
   `item_id`, `output_index`, and `content_index`.
4. On first tool-call index, emit `response.output_item.added` containing a
   `function_call`. Chat may split ID, name, and arguments across chunks; retain
   per-index state and emit argument deltas as
   `response.function_call_arguments.delta`.
5. At finish, emit argument `.done` and `response.output_item.done` for each tool
   call; emit `response.output_text.done`, `response.content_part.done`, and
   `response.output_item.done` for text.
6. Emit exactly one terminal `response.completed` or `response.incomplete`
   containing the fully assembled response and usage.

The Chat `[DONE]` sentinel is consumed and is not forwarded. Responses streams
terminate after the terminal semantic event; they do not require a `[DONE]`
sentinel. Usage-only Chat chunks are retained for terminal usage and do not
create output items. Malformed chunks, missing terminal state, inconsistent tool
indices, or upstream `error` data terminate with a Responses `error` event and a
failed stream status where possible.

## Errors, retries, and billing

- Pre-upstream conversion errors are client errors, use `SkipRetry`, and refund
  pre-consumed quota through the existing controller path.
- HTTP errors from the Chat upstream continue through `RelayErrorHandler` and
  status-code mapping.
- An error after SSE headers have been sent is emitted as a Responses stream
  error; it cannot change the HTTP status.
- Usage returned by Chat is the billing source. If absent, existing output-token
  estimation is used, with the original Responses prompt estimate as input.
- Quota settlement remains `PostTextConsumeQuota`; built-in-tool billing is not
  possible because built-in tools are rejected.

## TDD plan and test matrix

Tests are added in this order and observed failing before implementation:

1. Policy matching: disabled/default, all channels, channel ID/type, model regex.
2. Request conversion: string input, message arrays, explicit zeros/false,
   structured output, custom tools/tool choice, function-call history, images.
3. Request rejection: state, built-in tools, unsupported parts, malformed JSON.
4. Non-stream response conversion: text, tool calls, mixed output, usage,
   incomplete responses, invalid choice counts.
5. Stream conversion: text lifecycle, fragmented and parallel tool calls,
   usage-only chunk, incomplete finish, malformed input.
6. Relay integration with an `httptest` Chat upstream: URL switches to
   `/v1/chat/completions`, request body is Chat-shaped, client output remains
   Responses-shaped, and conversion chain is updated.
7. Regression: native Responses relay, Chat-via-Responses relay, DTO zero-value
   tests, and package-wide Go tests.

## Rollout

1. Ship with policy disabled.
2. Enable for one known Chat-only channel and a narrow model regex.
3. Compare success/error rate, first-token latency, usage totals, and retry rate.
4. Expand by channel type only after tool-call and streaming traffic is verified.
5. Disable the policy for immediate rollback; no schema migration is required.

## Official references

- OpenAI, “Migrate to the Responses API”:
  https://developers.openai.com/api/docs/guides/migrate-to-responses
- OpenAI, “Streaming API responses”:
  https://developers.openai.com/api/docs/guides/streaming-responses
- OpenAI, “Function calling”:
  https://developers.openai.com/api/docs/guides/function-calling
- OpenAI Responses API reference:
  https://developers.openai.com/api/docs/api-reference/responses
- OpenAI Chat Completions API reference:
  https://developers.openai.com/api/docs/api-reference/chat
