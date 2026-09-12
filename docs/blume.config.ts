import { defineConfig } from "blume";

export default defineConfig({
  title: "rolle",
  description: "Assume any role, any cloud. Short-lived AWS, Azure, and Google Cloud credentials for your tools, without a secret on disk.",
  // The wordmark is artwork, never typed: the lockup carries the mark and the word.
  logo: { image: { dark: "/logo-dark.svg", light: "/logo-light.svg", alt: "rolle" }, text: "", href: "/" },
  content: {
    sources: [
      { type: "filesystem", root: "content" },
      // Every GitHub release becomes an entry on /changelog and in its RSS
      // feed. release-please writes the release notes, so nothing is typed twice.
      { type: "github-releases", prefix: "changelog", owner: "nateships", repo: "rolle" },
    ],
  },
  github: { owner: "nateships", repo: "rolle", branch: "main", dir: "docs" },
  navigation: {
    actions: [
      { href: "/changelog", label: "Changelog" },
      { href: "https://github.com/nateships/rolle/releases", label: "Download" },
    ],
  },
  theme: { accent: "#00CE78", radius: "md", mode: "system" },
  ai: { llmsTxt: true },
  analytics: { vercel: true },
  seo: { og: { logo: "/mark.svg", palette: { background: "#101114", foreground: "#F4F0E8", muted: "#B4B8C0" } } },
  deployment: { output: "static", site: "https://getrolle.com" },
});
