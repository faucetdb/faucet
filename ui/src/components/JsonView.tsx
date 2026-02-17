interface JsonViewProps {
  data: unknown;
  collapsed?: boolean;
}

function syntaxHighlight(json: string): string {
  return json.replace(
    /("(\\u[a-zA-Z0-9]{4}|\\[^u]|[^\\"])*"(\s*:)?|\b(true|false|null)\b|-?\d+(?:\.\d*)?(?:[eE][+\-]?\d+)?)/g,
    (match) => {
      let cls = 'text-cyan-accent'; // number
      if (/^"/.test(match)) {
        if (/:$/.test(match)) {
          cls = 'text-brand-light'; // key
          match = match.replace(/:$/, '');
          return `<span class="${cls}">${match}</span>:`;
        } else {
          cls = 'text-success'; // string
        }
      } else if (/true|false/.test(match)) {
        cls = 'text-warning'; // boolean
      } else if (/null/.test(match)) {
        cls = 'text-text-muted'; // null
      }
      return `<span class="${cls}">${match}</span>`;
    }
  );
}

export function JsonView({ data }: JsonViewProps) {
  const json = JSON.stringify(data, null, 2);
  const highlighted = syntaxHighlight(json);

  return (
    <pre
      class="font-mono text-sm leading-relaxed overflow-auto p-4 bg-surface rounded-lg border border-border-subtle"
      dangerouslySetInnerHTML={{ __html: highlighted }}
    />
  );
}
