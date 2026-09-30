I'm writing the `capabilities` command for my agent-management CLI. Write its `main.go`.

It works out the goals the agents should pursue, turns each goal into a plan, then executes each plan. Executing a plan is flaky, so retry it a few times before giving up; I want to see the attempts, not a hidden loop. Each goal is its own visible step named after the goal.

I want live progress and a clear report. The domain helpers are in the package `evalsandbox/fixture`; call them, don't reimplement them.
