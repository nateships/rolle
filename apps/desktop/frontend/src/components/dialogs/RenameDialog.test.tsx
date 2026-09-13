import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { RenameDialog, type RenameTarget } from "@/components/dialogs/RenameDialog";

const target = (o: Partial<RenameTarget> = {}): RenameTarget => ({
  kind: "session",
  id: "s1",
  name: "personal",
  save: vi.fn().mockResolvedValue(undefined),
  ...o,
});

describe("RenameDialog", () => {
  it("saves a changed session name and closes", async () => {
    const user = userEvent.setup();
    const t = target();
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<RenameDialog target={t} onClose={onClose} />);

    expect(screen.getByRole("dialog", { name: "Rename session" })).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Save" });
    const input = screen.getByDisplayValue("personal");
    // The same name, or an empty one, cannot be saved.
    expect(save).toBeDisabled();
    await user.clear(input);
    expect(save).toBeDisabled();
    await user.type(input, "  work  ");
    expect(save).toBeEnabled();
    await user.click(save);

    await waitFor(() => expect(onClose).toHaveBeenCalledTimes(1));
    expect(t.save).toHaveBeenCalledWith("work");
    expect(success).toHaveBeenCalledWith("Renamed to work");
  });

  it("titles an integration rename as an account", () => {
    render(<RenameDialog target={target({ kind: "integration", name: "acme" })} onClose={vi.fn()} />);
    expect(screen.getByRole("dialog", { name: "Rename account" })).toBeInTheDocument();
    expect(screen.getByText("Only the display name changes.")).toBeInTheDocument();
  });

  it("lets a profile name be cleared back to default", async () => {
    const user = userEvent.setup();
    const t = target({ kind: "profile", name: "work" });
    const success = vi.spyOn(toast, "success");
    render(<RenameDialog target={t} onClose={vi.fn()} />);

    expect(screen.getByRole("dialog", { name: "AWS profile name" })).toBeInTheDocument();
    await user.clear(screen.getByDisplayValue("work"));
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(t.save).toHaveBeenCalledWith(""));
    expect(success).toHaveBeenCalledWith("Profile set to default");
  });

  it("names the new profile in the toast", async () => {
    const user = userEvent.setup();
    const success = vi.spyOn(toast, "success");
    render(<RenameDialog target={target({ kind: "profile", name: "" })} onClose={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("default"), "work");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(success).toHaveBeenCalledWith("Profile set to work"));
  });

  it("shows a save error and stays open", async () => {
    const user = userEvent.setup();
    const t = target({ save: vi.fn().mockRejectedValue(new Error("name in use")) });
    const error = vi.spyOn(toast, "error");
    const onClose = vi.fn();
    render(<RenameDialog target={t} onClose={onClose} />);

    await user.type(screen.getByDisplayValue("personal"), "2");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(error).toHaveBeenCalledWith("name in use"));
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("resets the field when the target changes", () => {
    const view = render(<RenameDialog target={target({ id: "a", name: "one" })} onClose={vi.fn()} />);
    expect(screen.getByDisplayValue("one")).toBeInTheDocument();
    view.rerender(<RenameDialog target={target({ id: "b", name: "two" })} onClose={vi.fn()} />);
    expect(screen.getByDisplayValue("two")).toBeInTheDocument();
  });

  it("renders nothing without a target", () => {
    render(<RenameDialog target={null} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });

  it("saves the role part as an alias for every account when the toggle is on", async () => {
    const user = userEvent.setup();
    const everywhere = {
      label: "Rename in every account",
      name: "ReadOnly",
      save: vi.fn().mockResolvedValue(undefined),
    };
    const t = target({ name: "Prod/ReadOnly", everywhere });
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<RenameDialog target={t} onClose={onClose} />);

    // The toggle swaps the field to the permission set alone; the unchanged
    // name cannot be saved.
    const toggle = screen.getByRole("switch", { name: "Rename in every account" });
    await user.click(toggle);
    const input = screen.getByDisplayValue("ReadOnly");
    expect(screen.getByRole("button", { name: "Save" })).toBeDisabled();
    // Off again restores the session name.
    await user.click(toggle);
    expect(screen.getByDisplayValue("Prod/ReadOnly")).toBeInTheDocument();
    await user.click(toggle);
    await user.clear(input);
    await user.type(input, "Viewer");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() => expect(everywhere.save).toHaveBeenCalledWith("Viewer"));
    expect(t.save).not.toHaveBeenCalled();
    expect(success).toHaveBeenCalledWith("ReadOnly is now Viewer in every account");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("warns about the typed profile name and offers the fix", async () => {
    const user = userEvent.setup();
    const check = vi.fn((name: string) =>
      Promise.resolve(name === "work" ? "Static keys in ~/.aws/credentials shadow this profile." : ""),
    );
    const onFix = vi.fn();
    render(<RenameDialog target={target({ kind: "profile", name: "", check, onFix })} onClose={vi.fn()} />);

    await user.type(screen.getByPlaceholderText("default"), " work ");
    const alert = await screen.findByRole("alert");
    expect(alert).toHaveTextContent("Static keys in ~/.aws/credentials shadow this profile.");
    // The fix gets the name as it would be saved.
    await user.click(screen.getByRole("button", { name: "Remove" }));
    expect(onFix).toHaveBeenCalledWith("work");
    // The warning follows the field: a clean name clears it.
    await user.type(screen.getByPlaceholderText("default"), "2");
    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
    // The check only ever saw trimmed names.
    for (const [name] of check.mock.calls) expect(name).toBe(name.trim());
  });

  it("drops the warning when the check itself fails", async () => {
    const user = userEvent.setup();
    // The field starts empty, so the first check sees ""; "a" is taken and
    // anything longer breaks the check.
    const check = vi.fn((name: string) => {
      if (name === "") return Promise.resolve("");
      if (name === "a") return Promise.resolve("taken");
      return Promise.reject(new Error("cannot read credentials"));
    });
    render(<RenameDialog target={target({ kind: "profile", name: "", check })} onClose={vi.fn()} />);
    const input = screen.getByPlaceholderText("default");
    await user.type(input, "a");
    expect(await screen.findByRole("alert")).toHaveTextContent("taken");
    // Without a fix handler the warning has no Remove button.
    expect(screen.queryByRole("button", { name: "Remove" })).not.toBeInTheDocument();
    await user.type(input, "b");
    await waitFor(() => expect(screen.queryByRole("alert")).not.toBeInTheDocument());
  });
});
