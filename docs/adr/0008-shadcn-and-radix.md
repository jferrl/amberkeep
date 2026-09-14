# 8. shadcn properly, and Radix where it earns it

- Status: accepted
- Date: 2026-09-14

## Context

The components under `web/src/components/ui/` were already shadcn's pattern: copied
into the repository rather than pulled from a registry, built on `cva`, `clsx` and
`tailwind-merge`. What was missing was the rest of it — a `components.json`, so the
shadcn CLI has somewhere to write, and the Radix primitives shadcn's own components
are usually built on.

Radix had been left out on two arguments. One of them was wrong.

The wrong one was the zero-network rule. That rule is about the running program:
Amberkeep fetches nothing at run time, from anywhere, ever. An npm package is
installed at build time and bundled; it makes no request when the page opens. Using
one takes nothing away from the rule, and citing it against a dependency was a
category error.

The argument that survives is narrower and still holds: a primitive is worth a
dependency when it does something the platform does not. Radix exists for focus
traps, ARIA plumbing and keyboard semantics — Dialog, Popover, Select, DropdownMenu,
Tooltip. This program has none of those. Every control in it is a native element, and
the accessibility assessment run over five screens found zero violations, precisely
because native elements get keyboard and screen-reader behaviour right for free.

So the question was not "Radix or not" but "which of these does the platform get
wrong".

## Decision

`components.json` is added, so `npx shadcn@latest add` works. **`shadcn init` is
deliberately not run**: it rewrites the stylesheet with its own token set, and this
project's palette was measured rather than picked — ink at 18.5:1, muted at 6.7:1,
control edges at 3.17:1 against the page. Losing that to a scaffold would undo a day
of work for a file it could not have known was load-bearing.

Two primitives, each for a reason the platform actually gives us:

**Checkbox.** The native one is not consistent across the two systems this ships to.
`accent-color` normalises the fill and nothing else; the box is a different size and
shape on macOS and on Windows. Both places this program uses one are screens where
somebody decides what happens to their whole history, and a control that looks
improvised there is one people hesitate over. Radix underneath rather than a styled
div, because the role, the checked state, the space key and the label association are
exactly what a div would have to reimplement and would get subtly wrong.

**Progress.** The wizard's step rule was a row of filled spans wearing `role="img"`
and an aria-label — a picture with a caption where a screen reader should have been
given a value. Radix gives `role="progressbar"` with a now and a max for the same
drawing.

Nothing else. No Dialog: the typed confirmation is a whole screen, and it should stay
one — the product register is explicit that a modal is usually laziness, and this is
the screen somebody consents on. No Select, Tooltip or DropdownMenu, because there is
nothing to put in them.

## Consequences

The bundle grows from 414 kB to 466 kB, about 12%. That is the honest price and it
buys two controls that behave correctly on both systems rather than one of them.

The verification is behavioural rather than visual: driven in a real browser, the
checkbox reports `role="checkbox"`, its `aria-checked` toggles, and the space key
works.

Adding a component later is now `npx shadcn@latest add <name>` followed by rewriting
its colours in this project's tokens — the CLI writes shadcn's own, which are not
ours. That is a small, repeatable edit, and it is the reason `init` stays unrun.
