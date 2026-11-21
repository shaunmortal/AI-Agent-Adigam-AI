import { useId } from 'react';

/**
 * The icons the app uses, as inline SVG paths.
 *
 * The glyphs are Lucide's (lucide.dev, ISC licence), flattened to one path each so the wrapper below
 * stays a single <path>. No icon library: `lucide-react` would ship far more than the few dozen used
 * here, and inline paths cost bytes only for the icons actually referenced.
 *
 * Lucide draws on a 24×24 grid; the stroke here is 1.75 so they sit at the same visual weight as the
 * type. The stroke inherits `currentColor`, so an icon in an active sidebar item turns white with
 * its label and never needs a second variant.
 */

export type IconName =
    | 'dashboard'
    | 'grid'
    | 'book'
    | 'network'
    | 'pencil'
    | 'listChecks'
    | 'clipboard'
    | 'calendar'
    | 'history'
    | 'gauge'
    | 'message'
    | 'users'
    | 'flag'
    | 'settings'
    | 'code'
    | 'file'
    | 'chart'
    | 'sparkles'
    | 'target'
    | 'shield'
    | 'lock'
    | 'flame'
    | 'chevron'
    | 'menu'
    | 'close'
    | 'mail'
    | 'eye'
    | 'eyeOff';

/** Path data only — the wrapper below supplies the shared attributes. */
const PATHS: Record<IconName, string> = {
    dashboard:
        'M4 3h5a1 1 0 0 1 1 1v7a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-7a1 1 0 0 1 1 -1zM15 3h5a1 1 0 0 1 1 1v3a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-3a1 1 0 0 1 1 -1zM15 12h5a1 1 0 0 1 1 1v7a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-7a1 1 0 0 1 1 -1zM4 16h5a1 1 0 0 1 1 1v3a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-3a1 1 0 0 1 1 -1z',
    grid:
        'M4 3h5a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1zM15 3h5a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1zM15 14h5a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1zM4 14h5a1 1 0 0 1 1 1v5a1 1 0 0 1 -1 1h-5a1 1 0 0 1 -1 -1v-5a1 1 0 0 1 1 -1z',
    book:
        'M12 5v16M20.001 19A2 2 0 0022 17V5a2 2 0 00-1.999-2L16 3.002A5 5 0 0012 5a5 5 0 00-4-2H4a2 2 0 00-2 2v12a2 2 0 001.999 2H8a5 5 0 014 2 5 5 0 014-2z',
    network:
        'M17 16h4a1 1 0 0 1 1 1v4a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1v-4a1 1 0 0 1 1 -1zM3 16h4a1 1 0 0 1 1 1v4a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1v-4a1 1 0 0 1 1 -1zM10 2h4a1 1 0 0 1 1 1v4a1 1 0 0 1 -1 1h-4a1 1 0 0 1 -1 -1v-4a1 1 0 0 1 1 -1zM5 16v-3a1 1 0 0 1 1-1h12a1 1 0 0 1 1 1v3M12 12V8',
    pencil:
        'M13 21h8M15 5l4 4M21.174 6.812a1 1 0 0 0-3.986-3.987L3.842 16.174a2 2 0 0 0-.5.83l-1.321 4.352a.5.5 0 0 0 .623.622l4.353-1.32a2 2 0 0 0 .83-.497z',
    listChecks:
        'M13 5h8M13 12h8M13 19h8M3 17l2 2 4-4M3 7l2 2 4-4',
    clipboard:
        'M9 2h6a1 1 0 0 1 1 1v2a1 1 0 0 1 -1 1h-6a1 1 0 0 1 -1 -1v-2a1 1 0 0 1 1 -1zM16 4h2a2 2 0 0 1 2 2v14a2 2 0 0 1-2 2H6a2 2 0 0 1-2-2V6a2 2 0 0 1 2-2h2M12 11h4M12 16h4M8 11h.01M8 16h.01',
    calendar:
        'M8 2v3M16 2v3M5 3h14a2 2 0 0 1 2 2v14a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-14a2 2 0 0 1 2 -2zM3 9h18M8 13h.01M12 13h.01M16 13h.01M8 17h.01M12 17h.01M16 17h.01',
    history:
        'M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8M3 3v5h5M12 7v5l4 2',
    gauge:
        'M12 14l4-4M3.34 19a10 10 0 1 1 17.32 0',
    message:
        'M2.992 16.342a2 2 0 0 1 .094 1.167l-1.065 3.29a1 1 0 0 0 1.236 1.168l3.413-.998a2 2 0 0 1 1.099.092 10 10 0 1 0-4.777-4.719',
    users:
        'M16 21v-2a4 4 0 0 0-4-4H6a4 4 0 0 0-4 4v2M16 3.128a4 4 0 0 1 0 7.744M22 21v-2a4 4 0 0 0-3-3.87M5 7a4 4 0 1 0 8 0a4 4 0 1 0 -8 0',
    flag:
        'M4 22V4a1 1 0 0 1 .4-.8A6 6 0 0 1 8 2c3 0 5 2 7.333 2q2 0 3.067-.8A1 1 0 0 1 20 4v10a1 1 0 0 1-.4.8A6 6 0 0 1 16 16c-3 0-5-2-8-2a6 6 0 0 0-4 1.528',
    settings:
        'M9.671 4.136a2.34 2.34 0 0 1 4.659 0 2.34 2.34 0 0 0 3.319 1.915 2.34 2.34 0 0 1 2.33 4.033 2.34 2.34 0 0 0 0 3.831 2.34 2.34 0 0 1-2.33 4.033 2.34 2.34 0 0 0-3.319 1.915 2.34 2.34 0 0 1-4.659 0 2.34 2.34 0 0 0-3.32-1.915 2.34 2.34 0 0 1-2.33-4.033 2.34 2.34 0 0 0 0-3.831A2.34 2.34 0 0 1 6.35 6.051a2.34 2.34 0 0 0 3.319-1.915M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0',
    code:
        'M16 18l6-6-6-6M8 6l-6 6 6 6',
    file:
        'M6 22a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h8a2.4 2.4 0 0 1 1.704.706l3.588 3.588A2.4 2.4 0 0 1 20 8v12a2 2 0 0 1-2 2zM14 2v5a1 1 0 0 0 1 1h5M10 9H8M16 13H8M16 17H8',
    chart:
        'M3 3v16a2 2 0 0 0 2 2h16M18 17V9M13 17V5M8 17v-3',
    sparkles:
        'M11.017 2.814a1 1 0 0 1 1.966 0l1.051 5.558a2 2 0 0 0 1.594 1.594l5.558 1.051a1 1 0 0 1 0 1.966l-5.558 1.051a2 2 0 0 0-1.594 1.594l-1.051 5.558a1 1 0 0 1-1.966 0l-1.051-5.558a2 2 0 0 0-1.594-1.594l-5.558-1.051a1 1 0 0 1 0-1.966l5.558-1.051a2 2 0 0 0 1.594-1.594zM20 2v4M22 4h-4M2 20a2 2 0 1 0 4 0a2 2 0 1 0 -4 0',
    target:
        'M2 12a10 10 0 1 0 20 0a10 10 0 1 0 -20 0M6 12a6 6 0 1 0 12 0a6 6 0 1 0 -12 0M10 12a2 2 0 1 0 4 0a2 2 0 1 0 -4 0',
    shield:
        'M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1zM9 12l2 2 4-4',
    lock:
        'M5 11h14a2 2 0 0 1 2 2v7a2 2 0 0 1 -2 2h-14a2 2 0 0 1 -2 -2v-7a2 2 0 0 1 2 -2zM7 11V7a5 5 0 0 1 10 0v4',
    flame:
        'M12 3q1 4 4 6.5t3 5.5a1 1 0 0 1-14 0 5 5 0 0 1 1-3 1 1 0 0 0 5 0c0-2-1.5-3-1.5-5q0-2 2.5-4',
    chevron:
        'M9 18l6-6-6-6',
    menu:
        'M4 5h16M4 12h16M4 19h16',
    close:
        'M18 6 6 18M6 6l12 12',
    mail:
        'M22 7l-8.991 5.727a2 2 0 0 1-2.009 0L2 7M4 4h16a2 2 0 0 1 2 2v12a2 2 0 0 1 -2 2h-16a2 2 0 0 1 -2 -2v-12a2 2 0 0 1 2 -2z',
    eye:
        'M2.062 12.348a1 1 0 0 1 0-.696 10.75 10.75 0 0 1 19.876 0 1 1 0 0 1 0 .696 10.75 10.75 0 0 1-19.876 0M9 12a3 3 0 1 0 6 0a3 3 0 1 0 -6 0',
    eyeOff:
        'M10.733 5.076a10.744 10.744 0 0 1 11.205 6.575 1 1 0 0 1 0 .696 10.747 10.747 0 0 1-1.444 2.49M14.084 14.158a3 3 0 0 1-4.242-4.242M17.479 17.499a10.75 10.75 0 0 1-15.417-5.151 1 1 0 0 1 0-.696 10.75 10.75 0 0 1 4.446-5.143M2 2l20 20',
};

export function Icon({
    name,
    size = 18,
    className,
}: {
    name: IconName;
    size?: number;
    className?: string;
}) {
    return (
        <svg
            width={size}
            height={size}
            viewBox="0 0 24 24"
            fill="none"
            stroke="currentColor"
            strokeWidth={1.75}
            strokeLinecap="round"
            strokeLinejoin="round"
            // Decorative in every current use: the label next to it already says what it means, and
            // a screen reader announcing "users icon, Groups" is noise rather than help.
            aria-hidden="true"
            focusable="false"
            className={className}
        >
            <path d={PATHS[name]} />
        </svg>
    );
}

/** The wordmark. An open book, because this is a study product and it reads at 20px. */
export function Logo({ size = 26 }: { size?: number }) {
    // One gradient id per logo: with a shared id every copy points at the first, and when that one
    // sits in a hidden column (the sign-in pitch on a phone) the others lose their stroke.
    const gradientId = `adigam-mark-${useId().replace(/[^a-zA-Z0-9_-]/g, '')}`;
    return (
        <svg
            width={size}
            height={size}
            viewBox="0 0 24 24"
            fill="none"
            aria-hidden="true"
            focusable="false"
        >
            <defs>
                <linearGradient id={gradientId} x1="0" y1="0" x2="24" y2="24">
                    <stop offset="0%" stopColor="#3b82f6" />
                    <stop offset="100%" stopColor="#8b5cf6" />
                </linearGradient>
            </defs>
            <path
                d="M3 5.5A1.5 1.5 0 014.5 4H10a2 2 0 012 2v13a2 2 0 00-2-2H4.5A1.5 1.5 0 013 15.5zM21 5.5A1.5 1.5 0 0019.5 4H14a2 2 0 00-2 2v13a2 2 0 012-2h5.5A1.5 1.5 0 0021 15.5z"
                stroke={`url(#${gradientId})`}
                strokeWidth={1.8}
                strokeLinecap="round"
                strokeLinejoin="round"
            />
        </svg>
    );
}
