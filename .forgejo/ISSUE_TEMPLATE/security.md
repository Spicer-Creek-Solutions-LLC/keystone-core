---
name: Security Vulnerability — DO NOT FILE PUBLICLY
about: Reporting a security issue? Read this first. Do not click through.
title: 'STOP — Security reports use the private channel in SECURITY.md'
labels: kind/security
assignees: ''
---

## STOP

**This is a public issue tracker. Security vulnerabilities must NOT
be filed as public issues.**

Public disclosure of an unpatched vulnerability puts every Keystone
Core operator at risk. Please use the private disclosure channel.

## How to report a security vulnerability

1. Close this template without submitting.
2. Follow the process in [`SECURITY.md`](../../SECURITY.md) — email the
   private disclosure address listed there with:
   - A description of the vulnerability
   - Steps to reproduce (or a proof of concept)
   - The potential impact you've identified
   - Any suggested fix, if you have one

Maintainers acknowledge security reports within ~72 hours
(per [`docs/project/MAINTAINERS.md`](../../docs/project/MAINTAINERS.md)
§ Triage and Response) and will coordinate disclosure with you.

## If you filed by mistake

If you've already submitted this issue: contact a maintainer privately
(see [`OWNERSHIP.md`](../../OWNERSHIP.md)) and we'll work with you to
take it down + coordinate proper disclosure. We won't penalize honest
mistakes — but please don't add further public detail in the
meantime.

## Why a redirect template

GitHub-style "private vulnerability reporting" exists on some forges
but not all. A redirect template here keeps the front door honest
across the forges Keystone Core publishes to (Codeberg primary,
GitHub code-only mirror).

---

**Do not submit this template.** Close it and follow
[`SECURITY.md`](../../SECURITY.md) instead.
