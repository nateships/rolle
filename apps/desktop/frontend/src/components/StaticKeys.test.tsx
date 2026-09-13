import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { StaticKeysCard } from "@/components/StaticKeys";
import { api } from "@/lib/api";

describe("StaticKeysCard", () => {
  it("renders nothing without static keys", async () => {
    vi.spyOn(api, "StaticProfiles").mockResolvedValue({ path: "~/.aws/credentials", profiles: [] });
    const { container } = render(<StaticKeysCard />);
    await waitFor(() => expect(api.StaticProfiles).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("lists the sections and removes one through the confirmation", async () => {
    const user = userEvent.setup();
    const list = vi
      .spyOn(api, "StaticProfiles")
      .mockResolvedValueOnce({ path: "~/.aws/credentials", profiles: ["default", "personal"] })
      .mockResolvedValue({ path: "~/.aws/credentials", profiles: ["personal"] });
    const remove = vi.spyOn(api, "RemoveStaticProfile").mockResolvedValue();
    render(<StaticKeysCard />);
    expect(await screen.findByText("default")).toBeInTheDocument();
    const [first] = screen.getAllByRole("button", { name: "Remove…" });
    await user.click(first);
    // Nothing goes before the confirmation.
    expect(remove).not.toHaveBeenCalled();
    expect(await screen.findByRole("dialog", { name: "Remove static keys?" })).toHaveTextContent("[default]");
    await user.click(screen.getByRole("button", { name: "Remove" }));
    await waitFor(() => expect(remove).toHaveBeenCalledWith("default"));
    await waitFor(() => expect(list).toHaveBeenCalledTimes(2));
    expect(screen.queryByText("default")).not.toBeInTheDocument();
    // Remove all shows for more than one section only.
    expect(screen.queryByRole("button", { name: "Remove all…" })).not.toBeInTheDocument();
  });
});
