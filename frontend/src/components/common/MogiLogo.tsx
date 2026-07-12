import "./MogiLogo.css";

interface Props {
  /** Rendered width/height in px. */
  size?: number;
  /**
   * "auto" (default) recolors with the theme — sumi ink on the light washi
   * theme, washi cream on indigo night. "cream" pins the cream inks for use
   * on a fixed indigo tile (the app-icon presentation), regardless of theme.
   */
  variant?: "auto" | "cream";
  className?: string;
}

// MogiLogo renders the Mogi brand mark — a tapering brush ensō circling a
// three-legged "m" gate, with a persimmon dot — as inline SVG so it stays
// crisp at any size and recolors through the CSS variables in MogiLogo.css.
export default function MogiLogo({ size = 32, variant = "auto", className }: Props) {
  const classes = [
    "mogi-mark",
    variant === "cream" ? "mogi-mark--cream" : "",
    className ?? "",
  ]
    .filter(Boolean)
    .join(" ");

  return (
    <svg
      className={classes}
      viewBox="0 0 100 100"
      width={size}
      height={size}
      xmlns="http://www.w3.org/2000/svg"
      role="img"
      aria-label="Mogi"
    >
      {/* Brush ensō: a single filled outline whose width tapers from a round
          brush-tip at the top right down to a hairline trailing end. */}
      <path
        className="mogi-mark-ring"
        d="M 88.11 43.28 L 88.56 46.15 L 88.80 49.06 L 88.82 51.98 L 88.62 54.89 L 88.19 57.79 L 87.56 60.65 L 86.70 63.46 L 85.64 66.20 L 84.37 68.86 L 82.91 71.42 L 81.25 73.86 L 79.41 76.18 L 77.40 78.35 L 75.24 80.37 L 72.92 82.23 L 70.46 83.90 L 67.88 85.39 L 65.20 86.69 L 62.42 87.78 L 59.56 88.66 L 56.64 89.32 L 53.67 89.77 L 50.68 89.99 L 47.67 89.98 L 44.67 89.75 L 41.69 89.29 L 38.75 88.61 L 35.86 87.71 L 33.04 86.59 L 30.32 85.27 L 27.69 83.74 L 25.19 82.01 L 22.82 80.10 L 20.60 78.01 L 18.53 75.76 L 16.64 73.36 L 14.93 70.82 L 13.42 68.15 L 12.11 65.37 L 11.01 62.50 L 10.12 59.55 L 9.46 56.54 L 9.03 53.48 L 8.83 50.40 L 8.86 47.30 L 9.12 44.21 L 9.61 41.15 L 10.34 38.13 L 11.29 35.16 L 12.46 32.27 L 13.85 29.48 L 15.45 26.79 L 17.24 24.23 L 19.23 21.80 L 21.39 19.53 L 23.73 17.43 L 26.21 15.50 L 28.84 13.76 L 31.60 12.22 L 34.47 10.90 L 37.43 9.79 L 40.47 8.90 L 43.57 8.25 L 46.72 7.83 L 49.90 7.64 L 53.08 7.70 L 56.26 7.99 L 59.41 8.53 L 62.51 9.30 L 65.55 10.30 L 68.51 11.53 L 71.37 12.98 A 4.75 4.75 0 0 1 66.62 21.20 L 64.44 19.99 L 62.17 18.94 L 59.82 18.06 L 57.40 17.35 L 54.94 16.83 L 52.44 16.50 L 49.92 16.36 L 47.39 16.40 L 44.86 16.64 L 42.36 17.06 L 39.89 17.67 L 37.47 18.47 L 35.12 19.45 L 32.84 20.61 L 30.65 21.93 L 28.56 23.42 L 26.59 25.06 L 24.74 26.85 L 23.03 28.78 L 21.46 30.83 L 20.06 33.00 L 18.81 35.27 L 17.74 37.64 L 16.85 40.08 L 16.15 42.58 L 15.63 45.14 L 15.31 47.73 L 15.18 50.34 L 15.24 52.96 L 15.51 55.57 L 15.97 58.15 L 16.62 60.70 L 17.47 63.20 L 18.50 65.63 L 19.72 67.98 L 21.10 70.23 L 22.66 72.38 L 24.38 74.41 L 26.25 76.31 L 28.25 78.06 L 30.39 79.66 L 32.64 81.10 L 35.00 82.36 L 37.46 83.45 L 39.99 84.35 L 42.58 85.06 L 45.23 85.58 L 47.91 85.89 L 50.61 86.00 L 53.32 85.91 L 56.01 85.61 L 58.68 85.12 L 61.31 84.42 L 63.89 83.53 L 66.39 82.44 L 68.81 81.16 L 71.13 79.71 L 73.33 78.08 L 75.42 76.29 L 77.36 74.35 L 79.16 72.26 L 80.79 70.04 L 82.26 67.70 L 83.55 65.25 L 84.65 62.71 L 85.56 60.08 L 86.27 57.40 L 86.78 54.66 L 87.08 51.89 L 87.18 49.10 L 87.06 46.30 L 86.73 43.52 A 0.70 0.70 0 0 1 88.11 43.28 Z"
      />
      <circle className="mogi-mark-dot" cx="80" cy="28" r="4.4" />
      <path className="mogi-mark-m" d="M 33 66 V 46 H 50 V 66 M 50 46 H 67 V 66" />
    </svg>
  );
}
