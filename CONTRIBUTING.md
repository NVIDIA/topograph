# Contribute to the NVIDIA `topograph` Project

Want to contribute to the NVIDIA `topograph` project? Awesome!
We only require you to sign your work as described in the following section.

For build, test, lint, and local-run commands, see the
[Development Guide](DEVELOPMENT.md).

## Open an issue first

Before opening a pull request, open an issue — this applies to bug fixes,
features, and any other change:

- **Bugs**: file a [bug report](https://github.com/NVIDIA/topograph/issues/new?template=bug_report.yml)
  describing what happened and what you expected instead.
- **Features or enhancements**: file a [feature request](https://github.com/NVIDIA/topograph/issues/new?template=feature_request.yml)
  describing the problem and your proposed solution.
- Search [existing issues](https://github.com/NVIDIA/topograph/issues) first to
  avoid duplicates, and comment on the issue to claim it before starting work.

Opening the issue first gives maintainers a chance to weigh in on approach
before code is written, and keeps the change from crossing one of this
project's load-bearing boundaries by accident:

- Changes to `pkg/topology/` (`Graph`, the `Vertex` tree, topology constants)
  must be discussed in an issue first — every provider and engine depends on
  this shape.
- Adding a new engine implies a new output format that every provider's
  output must be translatable into — coordinate with maintainers before
  starting.

Trivial fixes (typos, broken links, small doc corrections) don't need an
issue first — a pull request is fine.

## Sign your work

The sign-off is a simple signature at the end of the description for the patch.
Your signature certifies that you wrote the patch or otherwise have the right
to pass it on as an open-source patch.

The rules are pretty simple, and sign-off means that you certify the DCO below
(from [developercertificate.org](http://developercertificate.org/)):

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.
1 Letterman Drive
Suite D4700
San Francisco, CA, 94129

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.

Developer's Certificate of Origin 1.1

By making a contribution to this project, I certify that:

(a) The contribution was created in whole or in part by me and I
    have the right to submit it under the open source license
    indicated in the file; or

(b) The contribution is based upon previous work that, to the best
    of my knowledge, is covered under an appropriate open source
    license and I have the right under that license to submit that
    work with modifications, whether created in whole or in part
    by me, under the same open source license (unless I am
    permitted to submit under a different license), as indicated
    in the file; or

(c) The contribution was provided directly to me by some other
    person who certified (a), (b) or (c) and I have not modified
    it.

(d) I understand and agree that this project and the contribution
    are public and that a record of the contribution (including all
    personal information I submit with it, including my sign-off) is
    maintained indefinitely and may be redistributed consistent with
    this project or the open source license(s) involved.
```

To sign off, you just add the following line to every git commit message:

    Signed-off-by: Joe Smith <joe.smith@email.com>

You must use your real name (sorry, no pseudonyms or anonymous contributions).

If you set your `user.name` and `user.email` using git config, you can sign
your commit automatically with `git commit -s`.

## Commit messages

Use [Conventional Commits](https://www.conventionalcommits.org/):

```
type(scope): short description

optional body

Signed-off-by: Your Name <you@example.com>
```

`type` must be one of: `feat`, `fix`, `docs`, `chore`, `refactor`, `style`,
`perf`, `test`, `build`, `ci`. `scope` is optional and usually names the
package or area touched (e.g. `feat(provider/crusoe): ...`,
`fix(engine/k8s): ...`, `docs(contributing): ...`). Use the imperative mood
("add support for X", not "added support for X") and keep the summary line
under about 70 characters — it's what shows up in `git log --oneline` and in
the PR list.

Examples:

```
feat(provider/crusoe): add Crusoe Cloud provider

fix(engine/slurm): handle empty fabric tiers in topology.conf output

docs(providers/aws): document IAM permissions required for topology API
```

Branch names should use the same `type/` prefix as the commit
(`feat/`, `fix/`, `docs/`, `chore/`, `refactor/`, `test/`), e.g.
`feat/crusoe-provider`. Every commit still needs the DCO
`Signed-off-by:` trailer described above — `git commit -s` adds it for you.

## Community

Community discussion happens on the [Kubernetes Slack](https://slack.k8s.io/):

- [#topology-aware-scheduling](https://kubernetes.slack.com/archives/C012XSGFZQE) — topology-aware scheduling across the ecosystem
- [#gpu-nvidia](https://kubernetes.slack.com/archives/C09N46EFJR0) — NVIDIA GPU support on Kubernetes

For the project's current direction and a list of areas where contributions are especially welcome, see the pinned **Roadmap & Focus Areas** issue on the [issue tracker](https://github.com/NVIDIA/topograph/issues).
