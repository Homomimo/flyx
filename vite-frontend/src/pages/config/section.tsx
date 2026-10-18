import type * as React from "react";

import { ChevronDown } from "lucide-react";

/** One collapsible group of settings: title and current values, then fields. */
export function ConfigSection({
  id,
  title,
  summary,
  changed,
  open,
  onToggle,
  children,
}: {
  id: string;
  title: string;
  /** Current values, shown under the title. */
  summary?: React.ReactNode;
  /** Has edits that are not saved yet. */
  changed?: boolean;
  open: boolean;
  onToggle: () => void;
  children: React.ReactNode;
}) {
  return (
    <section
      className="scroll-mt-4 rounded-xl border border-divider bg-content1 shadow-sm"
      id={`config-${id}`}
    >
      <button
        aria-controls={`config-${id}-body`}
        aria-expanded={open}
        className="flex w-full items-center gap-3 px-3 py-3 text-left"
        type="button"
        onClick={onToggle}
      >
        <div className="min-w-0 flex-1">
          <h2 className="flex items-center gap-2 text-sm font-semibold">
            {title}
            {changed && (
              <span className="rounded-full bg-warning-100 px-1.5 py-0.5 text-[10px] font-normal text-warning-700 dark:bg-warning-900/40 dark:text-warning-300">
                未保存
              </span>
            )}
          </h2>
          {summary && (
            <p className="mt-0.5 truncate text-xs text-default-500">
              {summary}
            </p>
          )}
        </div>
        <ChevronDown
          className={`h-4 w-4 shrink-0 text-default-400 transition-transform ${
            open ? "rotate-180" : ""
          }`}
        />
      </button>
      {open && (
        <div
          className="space-y-4 border-t border-divider px-3 py-4"
          id={`config-${id}-body`}
        >
          {children}
        </div>
      )}
    </section>
  );
}

/** Label above the control with a line of help; `inline` puts the control on the right (switches). */
export function Field({
  label,
  help,
  inline,
  children,
}: {
  label: React.ReactNode;
  help?: React.ReactNode;
  inline?: boolean;
  children: React.ReactNode;
}) {
  if (inline) {
    return (
      <div className="flex items-center justify-between gap-3">
        <div className="min-w-0">
          <p className="text-sm font-medium">{label}</p>
          {help && <p className="text-xs text-default-500">{help}</p>}
        </div>
        <div className="shrink-0">{children}</div>
      </div>
    );
  }

  return (
    <div className="space-y-1.5">
      <p className="text-sm font-medium">{label}</p>
      {children}
      {help && <p className="text-xs text-default-500">{help}</p>}
    </div>
  );
}

/** Small heading inside a section, for a group of fields. */
export function SubHeading({ children }: { children: React.ReactNode }) {
  return (
    <p className="border-t border-divider pt-3 text-xs font-semibold text-default-500 first:border-t-0 first:pt-0">
      {children}
    </p>
  );
}
