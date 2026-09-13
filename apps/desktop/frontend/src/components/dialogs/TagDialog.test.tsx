import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { TagDialog } from "@/components/dialogs/TagDialog";
import { api } from "@/lib/api";
import { DEFAULT_TAG_COLOR } from "@/lib/tags";

describe("TagDialog", () => {
  it("creates a tag with the default glyph and a trimmed name", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddTag").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    const onClose = vi.fn();
    render(<TagDialog target={{ kind: "new" }} onClose={onClose} />);

    expect(screen.getByRole("dialog", { name: "New tag" })).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Save" });
    // A blank name cannot be saved.
    expect(save).toBeDisabled();
    await user.type(screen.getByLabelText("Name"), "  Prod  ");
    await user.click(save);

    await waitFor(() => expect(add).toHaveBeenCalledWith({ name: "Prod", color: DEFAULT_TAG_COLOR, icon: "tag" }));
    expect(success).toHaveBeenCalledWith("Tag Prod created");
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("edits a tag and reports the rename", async () => {
    const user = userEvent.setup();
    const update = vi.spyOn(api, "UpdateTag").mockResolvedValue();
    const onRenamed = vi.fn();
    render(
      <TagDialog
        target={{ kind: "edit", tag: { name: "Prod", color: "#244cff", icon: "shield" } }}
        onClose={vi.fn()}
        onRenamed={onRenamed}
      />,
    );
    expect(screen.getByRole("dialog", { name: "Edit tag" })).toBeInTheDocument();
    const input = screen.getByLabelText("Name");
    expect(input).toHaveValue("Prod");
    await user.clear(input);
    await user.type(input, "Production");
    await user.click(screen.getByRole("button", { name: "Save" }));

    await waitFor(() =>
      expect(update).toHaveBeenCalledWith("Prod", { name: "Production", color: "#244cff", icon: "shield" }),
    );
    // The sidebar filter follows the tag to its new name.
    expect(onRenamed).toHaveBeenCalledWith("Prod", "Production");
  });

  it("does not report a rename when only the look changes, and shows a save error", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "UpdateTag").mockRejectedValue(new Error("tag Production exists"));
    const error = vi.spyOn(toast, "error");
    const onRenamed = vi.fn();
    const onClose = vi.fn();
    render(
      <TagDialog
        target={{ kind: "edit", tag: { name: "Prod", color: "", icon: "" } }}
        onClose={onClose}
        onRenamed={onRenamed}
      />,
    );
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("tag Production exists"));
    expect(onRenamed).not.toHaveBeenCalled();
    expect(onClose).not.toHaveBeenCalled();
    expect(screen.getByRole("button", { name: "Save" })).toBeEnabled();
  });

  it("picks an icon and a color from the panel", async () => {
    const user = userEvent.setup();
    const add = vi.spyOn(api, "AddTag").mockResolvedValue();
    render(<TagDialog target={{ kind: "new" }} onClose={vi.fn()} />);

    const toggle = screen.getByRole("button", { name: "Choose icon and color" });
    expect(toggle).toHaveAttribute("aria-expanded", "false");
    await user.click(toggle);
    expect(toggle).toHaveAttribute("aria-expanded", "true");
    // The search narrows the grid; a click selects.
    await user.type(screen.getByLabelText("Search icons"), "shield-check");
    const icons = screen.getByRole("listbox", { name: "Icons" });
    await user.click(within(icons).getByRole("option", { name: "shield-check" }));
    expect(within(icons).getByRole("option", { name: "shield-check" })).toHaveAttribute("aria-selected", "true");
    await user.click(screen.getByRole("button", { name: "blue" }));
    expect(screen.getByRole("button", { name: "blue" })).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByLabelText("Color value")).toHaveValue("#244cff");
    // A typed color counts as custom.
    await user.clear(screen.getByLabelText("Color value"));
    await user.type(screen.getByLabelText("Color value"), "#123456");
    expect(screen.getByLabelText("Custom color")).toHaveAttribute("aria-pressed", "true");
    expect(screen.getByRole("button", { name: "blue" })).toHaveAttribute("aria-pressed", "false");
    // Nothing matches: the grid says so.
    await user.clear(screen.getByLabelText("Search icons"));
    await user.type(screen.getByLabelText("Search icons"), "zzzz-no-such-icon");
    expect(screen.getByText("No icon matches")).toBeInTheDocument();

    await user.type(screen.getByLabelText("Name"), "Secure");
    await user.click(screen.getByRole("button", { name: "Save" }));
    await waitFor(() => expect(add).toHaveBeenCalledWith({ name: "Secure", color: "#123456", icon: "shield-check" }));
  });

  it("renders nothing without a target and closes on Cancel", async () => {
    const user = userEvent.setup();
    const onClose = vi.fn();
    const view = render(<TagDialog target={null} onClose={onClose} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
    view.unmount();
    render(<TagDialog target={{ kind: "new" }} onClose={onClose} />);
    await user.click(screen.getByRole("button", { name: "Cancel" }));
    expect(onClose).toHaveBeenCalledTimes(1);
  });
});
