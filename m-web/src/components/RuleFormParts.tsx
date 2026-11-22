/**
 * Small pieces of the firewall-rule dialog (`FirewallRulesPanel.tsx`): the quiet label above each
 * part of the rule, and the row of buttons the action is picked from. They hold no state and know
 * nothing about rules beyond what is passed in.
 */
import type { ReactNode } from "react";
import { Button, ButtonGroup } from "@metap/ui";

/** The small caps label above a part of the form ("WHEN", "THEN"). `aside` sits at the right end
 *  of the same line, for the controls that belong to that part. */
export function SectionLabel({
  children,
  aside,
}: {
  children: ReactNode;
  aside?: ReactNode;
}) {
  return (
    <div className="mb-2 flex min-h-9 items-center justify-between gap-2">
      <h3 className="text-xs font-semibold uppercase tracking-wide text-muted-foreground">
        {children}
      </h3>
      {aside ? <div className="flex items-center gap-2">{aside}</div> : null}
    </div>
  );
}

export type ActionOption = { value: string; label: string; hint: string };

/** The action as one row of buttons with the chosen one's meaning written underneath: the four
 *  choices stay side by side and cost one line, where a card each would cost four. */
export function ActionPicker({
  value,
  onChange,
  options,
  label,
}: {
  value: string | undefined;
  onChange: (value: string) => void;
  options: ActionOption[];
  label: string;
}) {
  const selected = options.find((option) => option.value === value);
  return (
    <div className="grid gap-2">
      <ButtonGroup aria-label={label}>
        {options.map((option) => {
          const on = option.value === value;
          return (
            <Button
              key={option.value}
              type="button"
              aria-pressed={on}
              variant={
                on
                  ? option.value === "block"
                    ? "destructive"
                    : "default"
                  : "outline"
              }
              onClick={() => onChange(option.value)}
            >
              {option.label}
            </Button>
          );
        })}
      </ButtonGroup>
      {selected ? (
        <p className="text-xs text-muted-foreground">{selected.hint}</p>
      ) : null}
    </div>
  );
}
