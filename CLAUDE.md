# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

A CLI tool for Atlassian Confluence Cloud, built with Go for portability. Subcommands: `search`, `page`, `label`, `config`. Usage and configuration are in `README.md`.

## API Reference

- Uses Confluence REST API v2 (pages, labels, spaces) and v1 (CQL search, adding/removing labels)
- OpenAPI spec: `docs/openapi-v2.v3.json`
- API docs: https://developer.atlassian.com/cloud/confluence/rest/v2/intro#about

## Development

### Build and test

```bash
make build                         # ./confluence-cli
make install                       # go install .
make release-build                 # confluence-cli-<os>-<arch> for each platform
go vet ./... && go test ./...
go test -v ./internal/... -run TestName  # run single test
```

### E2E tests

`e2e/` runs the built binary against a real Confluence. It needs the `e2e` build tag and a test root page:

```bash
CONFLUENCE_E2E_ROOT=<page id> go test -tags e2e -count=1 ./e2e/...
```

- Pass the page id through the environment only. Never write it into files, tests, or commit messages.
- Write commands (`page create/update/delete`, `label add/remove`) must go through `env.write` in `e2e/helper_test.go`, which refuses any target that is not a descendant of the test root.
- Without `CONFLUENCE_E2E_ROOT` the tests skip.

### Public repository

Never put the company's Confluence site URL, real page ids, or internal page content in code, tests, testdata, docs, or commit messages. Use `example.atlassian.net` and fake ids. `testdata/private/` is gitignored.

## Architecture

Clean architecture. Dependencies point inward.

- `internal/domain`: types and errors shared by every layer
- `internal/cql`, `internal/storage`, `internal/convert/{tomd,frommd}`: pure logic (CQL building, storage-format XML, storage <-> Markdown)
- `internal/usecase`: commands' logic. Defines the port interfaces it calls (only the methods it uses) and the write guard (`read_only`, `writable_spaces`)
- `internal/adapter/{confluence,config,cache}`: implementations of the ports (HTTP client, config files and token command, cloudId cache)
- `internal/cli` (with `output`): cobra commands; wires adapters into usecases
- `main.go`: entry point
- `e2e/`: E2E tests (build tag `e2e`)

Dependency rules:

- `domain`, `cql`, `storage`, `convert/...`, `usecase` must not import cobra, `net/http`, or `os` (files and env).
- `adapter/...` must not import `usecase`.
- Interfaces live in the consuming package and have only the methods that package calls. No interface where there is one implementation and nothing is substituted in tests.
- Comments and docstrings are in Japanese and describe the current code only.
