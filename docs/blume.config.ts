import { defineConfig } from "blume";

export default defineConfig({
  title: "Rolle",
  description: "Assume any role, any cloud. Short-lived AWS, Azure, and Google Cloud credentials for your tools, without a secret on disk.",
  logo: { image: "/mark.svg", text: "Rolle", href: "/" },
  content: { root: "content" },
  github: { owner: "nateships", repo: "rolle", branch: "main", dir: "docs" },
  navigation: {
    actions: [{ href: "https://github.com/nateships/rolle/releases", label: "Download" }],
  },
  theme: { accent: "#00CE78", radius: "md", mode: "system" },
  ai: { llmsTxt: true },
  analytics: { vercel: true },
  deployment: { output: "static", site: "https://getrolle.com" },
});
