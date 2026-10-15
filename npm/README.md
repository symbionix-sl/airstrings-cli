# @airstrings/cli

CLI and MCP server for [AirStrings](https://airstrings.com): over-the-air string updates for iOS, Android, web and React Native apps, and Go backends. `airstrings publish` signs the bundle (Ed25519) and pushes it to the CDN; the SDKs verify the signature before showing anything.

```bash
npm install -g @airstrings/cli
```

Requires Node 18+. Installs two commands: `airstrings` (CLI) and `airstrings-mcp` (MCP server, stdio).

## Quick start

```bash
cd my-project
airstrings init                                   # log in, create a project, bind this folder
airstrings strings set welcome.title en="Hello" es="Hola" --format text
airstrings push                                   # send local strings to staging
airstrings publish                                # sign + publish staging bundles
airstrings sdk-config                             # IDs, public key and SDK snippets for production
```

Production is protected: changes reach it by promotion, applied by a human in the dashboard.

## MCP

```bash
airstrings mcp install                  # Claude Code
airstrings mcp install --claude-desktop # Claude Desktop
```

Other clients: run `airstrings-mcp` over stdio. Listed in the MCP Registry as `io.github.symbionix-sl/airstrings-cli`.

## Docs

- [CLI reference](https://github.com/symbionix-sl/airstrings-cli#readme)
- [MCP tools](https://github.com/symbionix-sl/airstrings-cli/blob/main/cmd/airstrings-mcp/README.md)
- `airstrings --help`, `airstrings <command> --help`

MIT licensed.
