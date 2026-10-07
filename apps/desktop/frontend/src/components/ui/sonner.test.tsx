import { act, render } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, describe, expect, it } from "vitest";
import { Toaster } from "@/components/ui/sonner";

describe("Toaster", () => {
  afterEach(() => document.documentElement.classList.remove("dark"));

  it("follows the theme when it changes after launch", async () => {
    document.documentElement.classList.remove("dark");
    const { container } = render(<Toaster />);
    // Sonner adds a toast on the next timer tick.
    await act(async () => {
      toast("Saved");
      await new Promise((r) => setTimeout(r, 0));
    });
    const list = container.querySelector("[data-sonner-toaster]");
    expect(list).toHaveAttribute("data-sonner-theme", "light");
    // The class observer reports in a microtask, which an async act flushes.
    await act(async () => document.documentElement.classList.add("dark"));
    expect(list).toHaveAttribute("data-sonner-theme", "dark");
  });
});
