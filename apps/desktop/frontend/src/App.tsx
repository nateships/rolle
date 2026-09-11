import { useState } from "react";
import { Button } from "@/components/ui/button";
import { GreetService } from "../bindings/github.com/nateships/rolle/apps/desktop";

export default function App() {
  const [name, setName] = useState("");
  const [greeting, setGreeting] = useState("");

  async function greet() {
    setGreeting(await GreetService.Greet(name || "Rolle"));
  }

  return (
    <main className="flex min-h-screen flex-col items-center justify-center gap-6 bg-background p-8 text-foreground">
      <h1 className="text-3xl font-semibold tracking-tight">Rolle</h1>
      <p className="text-muted-foreground">Assume any role, any cloud.</p>
      <div className="flex gap-2">
        <input
          className="h-9 rounded-md border border-input bg-transparent px-3 text-sm shadow-xs outline-none focus-visible:ring-2 focus-visible:ring-ring"
          placeholder="Your name"
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <Button onClick={greet}>Greet</Button>
      </div>
      {greeting && <p className="text-sm">{greeting}</p>}
    </main>
  );
}
