import { defineConfig } from "mygo-cli";

export default defineConfig({
  name: "Rokit Audio Editor",
  identifier: "com.znxae.rokit",
  version: "0.1.0",
  publisher: "znxae",
  // The installer is NSIS (see scripts/release.ps1); "Rokit" names its shortcuts, Apps entry and install folder.
  windows: { displayName: "Rokit", installDirName: "Rokit", installerInclude: "scripts/installer.nsh" },
  // Indonesian is the installer's language (the app's own text is written in the page).
  i18n: { dir: "locales", source: "id" },
  // `mygo dev` runs devCommand and loads devUrl; `mygo build` runs
  // buildCommand and embeds frontendDist into the app.
  devUrl: "http://localhost:5173",
  devCommand: "npm run dev:web",
  buildCommand: "npm run build:web",
  frontendDist: "dist",
  bindings: "src/mygo.ts",
  out: "build",
});
