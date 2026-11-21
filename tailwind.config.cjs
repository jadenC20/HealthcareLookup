module.exports = {
  "content": [
    "./index.html",
    "./app.js",
    "./pages/*.html"
  ],
  "theme": {
    "extend": {
      "colors": {
        "secondary-container": "#f0e8db",
        "primary": "#4a7c59",
        "surface-container-low": "#f5f1ea",
        "tertiary": "#705c30",
        "surface": "#faf6f0",
        "on-surface": "#2e3230",
        "on-error": "#ffffff",
        "outline-variant": "#c4c8bc",
        "on-tertiary": "#ffffff",
        "on-tertiary-fixed-variant": "#554020",
        "surface-bright": "#faf6f0",
        "background": "#faf6f0",
        "secondary-fixed-dim": "#d4ccbf",
        "on-tertiary-container": "#554020",
        "on-primary": "#ffffff",
        "primary-fixed": "#c8e8d0",
        "on-error-container": "#690005",
        "on-surface-variant": "#4a4e4a",
        "inverse-primary": "#8ecf9e",
        "surface-container-highest": "#e4e0d8",
        "on-primary-fixed": "#002110",
        "surface-tint": "#4a7c59",
        "on-secondary-fixed": "#1e1a13",
        "on-secondary-container": "#5e5548",
        "surface-variant": "#e4e0d8",
        "on-tertiary-fixed": "#221a05",
        "tertiary-fixed": "#f8e0a8",
        "on-secondary-fixed-variant": "#4a4538",
        "surface-container": "#f0ece4",
        "secondary-fixed": "#f0e8db",
        "tertiary-fixed-dim": "#dcc48e",
        "on-primary-container": "#d8f0de",
        "primary-fixed-dim": "#8ecf9e",
        "surface-container-high": "#eae6de",
        "primary-container": "#78a886",
        "secondary": "#6b6358",
        "inverse-surface": "#2e3230",
        "on-secondary": "#ffffff",
        "inverse-on-surface": "#f5f0e8",
        "on-primary-fixed-variant": "#2a6038",
        "error": "#b83230",
        "error-container": "#ffdad8",
        "outline": "#74796e",
        "surface-container-lowest": "#ffffff",
        "tertiary-container": "#c4a66a",
        "surface-dim": "#dbd7cf",
        "on-background": "#2e3230"
      },
      "borderRadius": {
        "DEFAULT": "0.75rem",
        "lg": "0.75rem",
        "xl": "0.75rem"
      },
      "fontFamily": {
        "headline": [
          "Nunito Sans",
          "sans-serif"
        ],
        "body": [
          "Nunito Sans",
          "sans-serif"
        ],
        "display": [
          "Literata",
          "serif"
        ],
        "label": [
          "Nunito Sans",
          "sans-serif"
        ]
      }
    }
  }
,
plugins: [require("@tailwindcss/forms"), require("@tailwindcss/container-queries")]
};
