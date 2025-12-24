lesser — Design Specification v0.1
1. Purpose & Tone
lesser is server-first, protocol-oriented infrastructure.
Its design should feel:
Bright, awake, and calm
Editorial rather than branded
Inevitable rather than expressive
Human-scale, not hype-driven
The UI should feel like daylight on paper, not software trying to be friendly.
2. Core Symbol
Primary Mark
/l/
Rendered directly from a serif font
No custom drawing or outlines
No color applied to the glyph itself
Meaning emerges from context, not decoration
Semantics
/ … l … / can be read as:
path syntax
vertical < | > (lesser / axis / greater)
server-contained namespace
Usage Rules
Unboxed: default, inline, textual contexts
Boxed: favicon, avatars, app icon, rare emphasis
The boxed version is a container, not a badge
3. Typography
Primary Typeface
Serif
Editorial / bookish
Clear slash and distinct lowercase l
Humanist or old-style preferred
Typography should:
Feel printable
Have visible rhythm
Avoid geometric perfection
Sans-serif may be used only for:
UI chrome
Code blocks
Dense data tables
4. Color Philosophy
Color is treated as illumination, not pigment.
The interface should feel lit by daylight, not painted.
5. Light Mode (Default)
Intent
“New day. Quiet room. Paper and ink.”
Palette Roles (not fixed colors)
Background
Cream-white
Slight warmth
Never pure white
Used for:
Primary surfaces
Reading contexts
Default pages
Foreground
Cool near-black
Ink / graphite tone
High legibility without harsh contrast
Used for:
Text
Icons
/l/ glyph
Accent / Interaction
Very pale daylight yellow
Only visible in contrast
Never used as body text
Used for:
Hover states
Focus rings
Selection backgrounds
Subtle separators
Rules
No gradients
No saturated colors
No “brand yellow” blocks
The accent should almost disappear when isolated
6. Dark Mode (Secondary, Still “Day”)
Dark mode is not night — it is interior light.
Intent
“Evening room. Lamps. Paper still exists.”
Dark Palette Roles
Background
Warm charcoal
Not pure black
Slight brown or umber undertone
This avoids:
OLED harshness
Hacker aesthetic
Night-mode clichés
Foreground
Soft off-white
Not full white
Feels like paper under low light
Accent / Interaction
Muted candlelight yellow
Lower saturation than light mode
Used sparingly
Used for:
Focus
Active states
Selection
Important but non-alarming emphasis
Dark Mode Rules
/l/ remains neutral (light on dark)
No neon accents
No blue highlights
Contrast should feel comfortable, not sharp
If light mode is “morning,” dark mode is “still awake.”
7. Contrast & Accessibility
Favor moderate contrast, not maximum
Avoid pure black-on-white or white-on-black
Text should pass accessibility, but:
Use weight, spacing, and hierarchy before color
Accent color should never be the only signal
8. UI Hierarchy
Priority order:
Typography
Spacing
Light
Color (last)
If something needs color to be understood, it probably needs rethinking.
9. What lesser is not
Not playful
Not loud
Not gradient-driven
Not AI-branded
Not dark-first
Not “friendly SaaS”
Calm ≠ boring. Quiet ≠ empty.