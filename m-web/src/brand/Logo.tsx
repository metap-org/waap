import { useId } from "react";

/** The WAAP mark: a shield with a traffic line running through it and a node where it is
 *  inspected. The shield says protection; the line says it is watching live traffic, which is the
 *  product. Drawn in `currentColor` for the shield and the theme's signal color for the line, so it
 *  follows the rail it sits on and the active theme without separate assets. */
export function LogoMark({ className = "h-6 w-6" }: { className?: string }) {
  const clip = useId();
  return (
    <svg
      viewBox="0 0 32 32"
      className={className}
      fill="none"
      aria-hidden
      focusable="false"
    >
      <defs>
        <clipPath id={clip}>
          <path d="M16 3.2 5.2 7.1v8.1c0 6.2 4.4 11.1 10.8 13.6 6.4-2.5 10.8-7.4 10.8-13.6V7.1L16 3.2Z" />
        </clipPath>
      </defs>
      <path
        d="M16 3.2 5.2 7.1v8.1c0 6.2 4.4 11.1 10.8 13.6 6.4-2.5 10.8-7.4 10.8-13.6V7.1L16 3.2Z"
        fill="currentColor"
        fillOpacity="0.12"
        stroke="currentColor"
        strokeWidth="1.6"
        strokeLinejoin="round"
      />
      <g clipPath={`url(#${clip})`}>
        <path
          d="M3 17.5h6.2l2.3-5.4 3.4 10 2.6-6.4 1.3 1.8H29"
          stroke="hsl(var(--sidebar-primary))"
          strokeWidth="1.8"
          strokeLinecap="round"
          strokeLinejoin="round"
        />
      </g>
      <circle cx="16" cy="16.9" r="1.7" fill="hsl(var(--sidebar-primary))" />
    </svg>
  );
}

/** Mark plus the name, for the navigation rail. The name is set small-caps with wide tracking so
 *  it reads as a product mark rather than a heading. */
export function Brand({ name = "WAAP" }: { name?: string }) {
  return (
    <>
      <LogoMark className="h-6 w-6 shrink-0" />
      <span className="truncate text-[13px] font-bold uppercase tracking-[0.18em]">
        {name}
      </span>
    </>
  );
}
