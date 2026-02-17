import { useState } from 'preact/hooks';

interface CodeBlockProps {
  code: string;
  language?: string;
}

export function CodeBlock({ code, language = '' }: CodeBlockProps) {
  const [copied, setCopied] = useState(false);

  const handleCopy = async () => {
    try {
      await navigator.clipboard.writeText(code);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback
      const textarea = document.createElement('textarea');
      textarea.value = code;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand('copy');
      document.body.removeChild(textarea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  };

  return (
    <div class="relative group rounded-lg border border-border-subtle bg-surface overflow-hidden">
      {/* Language label */}
      {language && (
        <div class="px-4 py-1.5 border-b border-border-subtle text-xs font-mono text-text-muted bg-surface-raised">
          {language}
        </div>
      )}

      {/* Copy button */}
      <button
        onClick={handleCopy}
        class={`
          absolute top-2 right-2 px-2 py-1 rounded text-xs font-medium
          transition-all duration-150
          ${copied
            ? 'bg-success/20 text-success'
            : 'bg-surface-overlay text-text-muted hover:text-text-primary opacity-0 group-hover:opacity-100'
          }
        `}
      >
        {copied ? (
          <span class="flex items-center gap-1">
            <svg class="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
              <path fill-rule="evenodd" d="M16.707 5.293a1 1 0 010 1.414l-8 8a1 1 0 01-1.414 0l-4-4a1 1 0 011.414-1.414L8 12.586l7.293-7.293a1 1 0 011.414 0z" clip-rule="evenodd" />
            </svg>
            Copied
          </span>
        ) : (
          <span class="flex items-center gap-1">
            <svg class="w-3.5 h-3.5" viewBox="0 0 20 20" fill="currentColor">
              <path d="M8 3a1 1 0 011-1h2a1 1 0 110 2H9a1 1 0 01-1-1z" />
              <path d="M6 3a2 2 0 00-2 2v11a2 2 0 002 2h8a2 2 0 002-2V5a2 2 0 00-2-2 3 3 0 01-3 3H9a3 3 0 01-3-3z" />
            </svg>
            Copy
          </span>
        )}
      </button>

      {/* Code content */}
      <pre class="p-4 overflow-x-auto text-sm font-mono leading-relaxed text-text-primary">
        <code>{code}</code>
      </pre>
    </div>
  );
}
