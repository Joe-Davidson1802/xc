---
title: "Requires"
description:
linkTitle: "Requires"
menu: { main: { parent: 'task-syntax', weight: 10 } }
---

## Task Dependencies

The `requires` attribute can be used to define tasks that should be run prior to another task.

## Syntax

For example, you want to run a task to deploy your code, but you want to run all the unit tests and linters before that happens.

Before the command section of the task, you may define comma separated dependencies with `requires:` or `req:` followed by the name of the tasks that are dependencies.

````markdown
## Tasks

### Test
```
sh test.sh
```

### Lint
```
sh lint.sh
```

### Deploy
requires: Test, Lint
```
sh deploy.sh
```
````

## Chaining tasks with dependencies

You can chain tasks through the use of dependencies.

````markdown
## Tasks

### Task1
```
echo "TASK 1"
```

### Task2
requires: Task1
```
echo "TASK 2"
```

### Task3
requires: Task2
```
echo "TASK 3"
```
````

Running `xc Task3` will yield:

```
echo "TASK 1"
TASK 1
echo "TASK 2"
TASK 2
echo "TASK 3"
TASK 3
```

Running in the order of `Task1` -> `Task2` -> `Task`

## Modifying required task behaviour

See [Run](/task-syntax/run/)
