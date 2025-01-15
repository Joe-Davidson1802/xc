---
title: "Run Dependencies"
description:
linkTitle: "RunDeps"
menu: { main: { parent: "task-syntax", weight: 11 } }
---

## RunDeps attribute

By default, the dependencies of a task are run sequentially, in the order they are listed.  
However we may prefer for all the dependencies of a task to be run in parallel.

The solution would be to set the `RunDeps` attribute to `async` (defaults to `sync`).

```markdown
### build-all

Requires: build-js, build-css

RunDeps: async
```

This will result in both `build-js` and `build-css` being run in parallel.

The default is `sync`, which can be omitted or specified.

```markdown
### build-all

Requires: build-js, build-css

RunDeps: sync
```

is the same as

```markdown
### build-all

Requires: build-js, build-css
```
