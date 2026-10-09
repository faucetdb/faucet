import { JSX } from 'preact';

/**
 * Syntax-highlighted JSON. Values come from the user's database, so this
 * builds elements rather than an HTML string: nothing in the data can be
 * interpreted as markup.
 */
const TOKEN = /("(?:\\u[a-fA-F0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(?:true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+-]?\d+)?)/g;

export function JsonView({ data, class: cls = '' }: { data: unknown; class?: string }) {
  const json = JSON.stringify(data, null, 2) ?? 'undefined';
  const parts: (string | JSX.Element)[] = [];
  let last = 0;
  let i = 0;
  for (const m of json.matchAll(TOKEN)) {
    const idx = m.index ?? 0;
    if (idx > last) parts.push(json.slice(last, idx));
    const tok = m[0];
    let color = 'text-[var(--live)]'; // number
    let text = tok;
    let suffix = '';
    if (tok.startsWith('"')) {
      if (m[2]) {
        color = 'text-brand-fg'; // key
        text = tok.slice(0, tok.lastIndexOf('"') + 1);
        suffix = tok.slice(text.length);
      } else {
        color = 'text-ok'; // string
      }
    } else if (tok === 'true' || tok === 'false') {
      color = 'text-warn';
    } else if (tok === 'null') {
      color = 'text-fg-faint';
    }
    parts.push(<span key={i++} class={color}>{text}</span>);
    if (suffix) parts.push(suffix);
    last = idx + tok.length;
  }
  if (last < json.length) parts.push(json.slice(last));

  return (
    <pre class={`font-mono text-[12.5px] leading-[20px] text-fg overflow-auto ${cls}`}>
      <code>{parts}</code>
    </pre>
  );
}
