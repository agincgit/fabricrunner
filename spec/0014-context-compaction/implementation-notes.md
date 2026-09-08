# Implementation decisions

`ModelRequest.AutomaticCompaction` is an explicit runner control. A model's
capability advertisement never enables it. `Engine.ContextCounter` supplies
model-specific input token accounting; missing accounting fails closed when a
candidate declares a context window. A conservative serialized-byte counter is
available for applications whose tokenizer/framing contract permits it.

Compaction starts at 80% of the smallest declared candidate window. It uses an
ordinary policy-checked, routed, reserved model call, with tool use disabled.
The input is the bounded, classification-filtered history plus a summarization
instruction. The summary and exact inputs, excluded message indexes, selected
model, usage, token counts and replaced durable event range are recorded before
the loop adopts the replacement history. A request too large even for the
summarization call fails explicitly; this initial implementation does not
silently truncate history or recursively compact until a budget runs out.

The loop accepts replacement messages in `TurnSelection.Messages`. The engine
hashes and authorizes that replacement before the normal provider call. Actual
reported context usage remains in model usage events; estimated context counts
are identified separately. Resume must apply the committed checkpoint/summary
and must not re-run a completed compaction.
