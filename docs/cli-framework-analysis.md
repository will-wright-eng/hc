# CLI Framework Analysis: Cobra vs urfave/cli v3

## Purpose

Evaluate two Go CLI frameworks for building `hc`, a hot/cold codebase analysis tool. The primary selection criterion is minimizing framework friction — avoiding the "fighting the framework" problem where the tool's design is constrained by the framework's opinions rather than the problem domain.

---

## Cobra (github.com/spf13/cobra)

### Overview

Cobra is the dominant Go CLI framework, used by Kubernetes, Hugo, and the GitHub CLI. It provides an imperative command-tree model with a companion code generator (`cobra-cli`) and deep integration with Viper for configuration.

### Design Model

Commands are defined as `*cobra.Command` struct literals and wired together via `AddCommand()` calls. The idiomatic pattern uses `init()` functions across multiple files, with one command per file in a `cmd/` package:

```go
var rootCmd = &cobra.Command{Use: "myapp", ...}
var serveCmd = &cobra.Command{Use: "serve", RunE: serveFn}

func init() { rootCmd.AddCommand(serveCmd) }
```

Flags delegate to the `spf13/pflag` library. Binding is done via method calls: `cmd.Flags().StringVarP(&myVar, "name", "n", "default", "help")`. Required flags and mutual exclusion are declared by string name: `cmd.MarkFlagRequired("name")`.

### Strengths

- Mature ecosystem with extensive documentation and community examples
- Built-in positional argument validation (`ExactArgs`, `MinimumNArgs`, etc.)
- Shell completion generation out of the box
- File-per-command convention scales well for large CLIs (50+ subcommands)

### Friction Points

- **Distributed command tree.** The `init()` wiring pattern scatters the CLI structure across files. The full command tree is never visible in one place — it is assembled at startup through side effects.
- **String-based flag references.** `MarkFlagRequired("naem")` silently does nothing. Typos in flag names are runtime errors with no compile-time safety.
- **Lifecycle hook proliferation.** 10 hook fields on the Command struct (`Pre/Post/Persistent` x `Run/RunE`). For a simple tool, this is unnecessary cognitive overhead.
- **Opinionated scaffolding.** The generator and tutorials push toward a `cmd/` layout with Viper integration. Deviating from this convention means working against the grain of most examples and documentation.
- **External dependencies.** Brings in pflag, go-md2man, mousetrap, and yaml as transitive dependencies.

---

## urfave/cli v3 (github.com/urfave/cli/v3)

### Overview

urfave/cli is a long-standing Go CLI library (24k+ stars) that takes a declarative, struct-based approach. v3 is a significant rewrite that removed `cli.App` and `cli.Context`, unifying everything under `*cli.Command`.

### Design Model

The entire command tree is a single nested struct literal. Subcommands are fields, flags are fields, actions are fields:

```go
cmd := &cli.Command{
    Name: "hc",
    Commands: []*cli.Command{
        {Name: "analyze", Flags: []cli.Flag{...}, Action: analyzeFn},
    },
}
cmd.Run(context.Background(), os.Args)
```

No `init()` functions, no `AddCommand` calls, no prescribed file layout. Flag types are structs (`&cli.StringFlag{}`, `&cli.BoolFlag{}`) with inline configuration for defaults, environment variable sources, and required status.

### Strengths

- **Zero external dependencies** — stdlib only
- **Declarative tree** — the full CLI shape is visible in one struct literal
- **Minimal API surface** — 2 lifecycle hooks (`Before`, `After`) vs cobra's 10
- **Low boilerplate** — a 2-subcommand CLI with flags fits in ~35 lines
- **No scaffolding or codegen** — no prescribed directory structure

### Friction Points

- **No built-in positional argument validation.** Must manually check `cmd.NArg()` in the action function. Trivial to implement but not provided.
- **v3 API stability.** v3 is relatively new and the API has shifted across minor releases (v3.6–v3.8). The v2-to-v3 migration was a breaking change.
- **Default `-v` flag conflict.** The built-in `--version` / `-v` flag can conflict with `-v` for verbose mode. Requires explicit override.
- **String-based flag retrieval.** `cmd.String("naem")` returns a zero value silently if the flag name is wrong. An `InvalidFlagAccessHandler` callback exists but is opt-in.

---

## Comparison Summary

| Dimension | Cobra | urfave/cli v3 |
|---|---|---|
| Command definition | Imperative (`AddCommand`) | Declarative (nested structs) |
| Tree visibility | Distributed across files via `init()` | Single struct literal |
| Flag system | External (pflag) | Built-in, zero deps |
| Lifecycle hooks | 10 | 2 |
| Arg validation | Built-in | Manual |
| Dependencies | 4 external | 0 |
| Boilerplate (2 subcmds) | ~55 lines / 4 files | ~35 lines / 1 file |
| Adoption | 43k+ stars, kubectl/hugo/gh | 24k+ stars, long history |

## Recommendation

**urfave/cli v3** is the better fit for this project. The CLI is small (2-3 subcommands), the problem domain is well-scoped, and the primary concern is avoiding framework overhead. Cobra's organizational patterns are designed for large, complex CLIs — for a focused analysis tool, they add ceremony without proportional benefit. urfave's declarative model keeps the CLI definition readable and the dependency footprint at zero.
