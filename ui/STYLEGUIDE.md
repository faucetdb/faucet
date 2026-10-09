# Faucet admin UI: design notes

These notes cover the admin UI that ships inside the `faucet` binary. Read them before you change a page.

## What it is for

Faucet turns a database into a REST API and an MCP server. The admin UI exists to do four things fast:

1. Connect a database.
2. Decide who can do what.
3. Hand out a key.
4. Make a first request, or connect an AI agent.

The audience is developers on solo projects and small teams, often running Faucet next to an on-prem database. Everything the UI needs is bundled, so it works air-gapped: no CDN fonts and no remote images.

## Visual language: "deep water"

The palette matches the brand on faucetdb.ai: navy surfaces, electric blue, and cyan for anything flowing or live. There are light and dark themes. Dark is the default, and "system" is honored. All colors are CSS variables set in `src/index.css`, and Tailwind utilities map to them. **Never hard-code a hex value in a component.**

| Token | Use |
| --- | --- |
| `bg` | Page background |
| `panel` | Sidebar, tables, drawers, raised regions |
| `panel-2` | Hover, input wells, code blocks |
| `line` / `line-strong` | Hairlines between regions, input borders |
| `fg` / `fg-muted` / `fg-faint` | Text: primary, secondary, placeholder/disabled |
| `brand` (+ `brand-fg` for text on tinted backgrounds) | Primary actions, selection, links |
| `live` | Only for "connected / flowing / live" states and the flow line |
| `ok` / `warn` / `bad` | Status |

### Type

- **Inter Variable** for the interface.
- **JetBrains Mono Variable** only for literal machine text: URLs, table and column names, keys, DSNs, JSON, and SQL types. Never use mono for decoration or for small labels.

The scale is 12 / 13 / 14 (base) / 16 / 20 / 26. Use weights 400, 500 and 600 only.

### Shape

- Radii follow hierarchy: 6px for controls, 10px for panels, 14px for drawers and modals.
- Shadows only on things that float (drawer, modal, toast, menu).
- Regions are separated by hairlines, not shadows.

### Motion

- Motion answers the user: a drawer slides in, a toast appears, a row is highlighted after save.
- There is one signature moment. When a connection test succeeds, the flow line in the connect drawer "turns on": cyan dashes travel from the database to the endpoint.
- Everything respects `prefers-reduced-motion`.

### The flow line (signature)

The flow line is `<FlowLine>`: database ─── Faucet ─── `/api/v1/{name}` + MCP. It shows exactly what Faucet does. It appears in the connect drawer and in setup. Don't use it anywhere else.

## Rules that keep it from looking generated

- Sentence case everywhere: no ALL-CAPS badges or labels, no tracked-out eyebrows above headings.
- No "→" appended to links or buttons, and no "A · B · C" meta strings.
- Numbered markers only for real sequences. The getting-started checklist and the setup steps qualify.
- Don't chop every page into identical cards. Lists of things are rows in a panel with hairlines between them. Use cards only where items are genuinely independent.
- Left-align content. Each page has one primary action, on the right of the page header.

## Words

- Name things the way users think of them:
  - "Databases", not "services". The URL slug is the "API name".
  - "Read-only", not "verb mask GET".
- Buttons say what happens ("Add database", "Test connection", "Create key"). The matching toast uses the same verb ("Database added").
- Errors say what happened and what to do next. They never apologize.
- An empty state is an invitation to act, and offers the one action that fills it.

## Building blocks (`src/components`)

| Component | What it does |
| --- | --- |
| `Button` | `variant`: `primary`, `secondary`, `ghost`, `danger`; `size`: `sm`, `md` |
| `Field`, `Input`, `Select`, `Textarea`, `Switch`, `Segmented` | Form controls, each with a label, hint and error slot |
| `Drawer`, `Modal`, `confirm()` | Overlays. `confirm()` is promise-based and replaces `window.confirm` |
| `toast()` | Transient feedback |
| `PageHeader` | Title, description and actions |
| `Panel` | Bordered region, optionally with a header |
| `EmptyState` | Empty-state block |
| `CodeBlock`, `CopyButton` | Copyable code |
| `DbLogo` | Database logo for a driver id |
| `StatusDot` | Status indicator |
| `Tabs` | Tab strip |
| `Spinner` | Loading indicator |
| `JsonView` | Escaped, highlighted JSON |
| `FlowLine` | The signature flow line (see above) |

Database metadata (labels, default ports, field sets, TLS options) lives in `src/lib/drivers.ts`. Formatting helpers live in `src/lib/format.ts`.
