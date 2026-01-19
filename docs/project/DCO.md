# Developer Certificate of Origin

Version 1.1 (AI-Aware)

By making a contribution to this project, I certify that:

**(a)** The contribution was created in whole or in part by me and I have the
right to submit it under the open source license indicated in the file; or

**(b)** The contribution is based upon previous work that, to the best of my
knowledge, is covered under an appropriate open source license and I have the
right under that license to submit that work with modifications, whether
created in whole or in part by me, under the same open source license (unless
I am permitted to submit under a different license), as indicated in the file;
or

**(c)** The contribution was provided directly to me by some other person who
certified (a), (b) or (c) and I have not modified it.

**(d) AI-Generated Content**: If this contribution includes content generated
by artificial intelligence tools, I additionally certify that:

   - I have disclosed the use of AI assistance in the commit message using the
     format specified in CONTRIBUTING.md
   - I have reviewed and tested the AI-generated content
   - I understand that AI-generated content may not be eligible for copyright
     protection under current law
   - I take full responsibility for the contribution as if I had authored it
     myself
   - I make no copyright claim on portions that are purely AI-generated without
     my creative input
   - I have verified the AI-generated content does not knowingly infringe on
     any third party's intellectual property rights

**(e)** I understand and agree that this project and the contribution are
public and that a record of the contribution (including all personal
information I submit with it, including my sign-off) is maintained indefinitely
and may be redistributed consistent with this project or the open source
license(s) involved.

---

## How to Sign Off

Add a `Signed-off-by` line to your commit messages:

```
Signed-off-by: Your Name <your.email@example.com>
```

You can do this automatically with `git commit -s`.

### Example Commit (Human-Authored)

```
feat(state): add drift detection for file modules

Implement content-based drift detection for file state modules,
comparing actual file content against declared state.

Signed-off-by: Jane Developer <jane@example.com>
```

### Example Commit (AI-Assisted)

```
feat(state): add drift detection for file modules

Implement content-based drift detection for file state modules,
comparing actual file content against declared state.

🤖 Generated with [Claude Code](https://claude.com/claude-code)

Co-Authored-By: Claude <noreply@anthropic.com>
Signed-off-by: Jane Developer <jane@example.com>
```

---

## About This DCO

This DCO is based on the standard [Developer Certificate of Origin v1.1](https://developercertificate.org/)
used by the Linux kernel and many other open source projects, with the addition
of clause (d) to address AI-generated contributions.

The standard DCO text is:

```
Developer Certificate of Origin
Version 1.1

Copyright (C) 2004, 2006 The Linux Foundation and its contributors.

Everyone is permitted to copy and distribute verbatim copies of this
license document, but changing it is not allowed.
```

Our modifications (clause d) are additions that do not change the original
terms, only extend them to cover the novel case of AI-assisted contributions.
