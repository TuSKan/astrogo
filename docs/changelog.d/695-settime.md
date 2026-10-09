---
type: Changed — BREAKING
pr: 695
---
**`coord.Context.SetTime` replaces `AtTime` and `Clone`**: it moves a Context in place without allocating, and rebuilds it an hour from its epoch, so its error stays ≲0.1″ for every caller. `AtTime` copied 704 bytes per instant, 72% of what plan's event solver allocated. A copy is `c := *ctx`. `plan.TransitionContext`, since renamed `plan.Transition`, carries both targets' observed alt/az instead of `ContextAt`; `plan.NewTransition` builds one (#675).
