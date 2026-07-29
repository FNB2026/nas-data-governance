import { useState, useCallback } from "react";
import { copyToClipboard } from "../lib/utils";

export interface CopyButtonProps {
  text: string;
  label?: string;
  className?: string;
}

export default function CopyButton({ text, label = "复制内容", className = "" }: CopyButtonProps) {
  const [copied, setCopied] = useState(false);

  const handleClick = useCallback(async () => {
    const ok = await copyToClipboard(text);
    if (ok) {
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  }, [text]);

  return (
    <button
      className={`btn-sm copy-btn ${className}`}
      type="button"
      onClick={() => void handleClick()}
      title={copied ? "已复制" : label}
      aria-label={copied ? `${label}成功` : label}
    >
      {copied ? "✓" : "⧉"}
      <span className="sr-only" aria-live="polite">{copied ? "已复制" : ""}</span>
    </button>
  );
}
