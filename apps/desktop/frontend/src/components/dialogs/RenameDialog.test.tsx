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
});
