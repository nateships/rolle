import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it, vi } from "vitest";
import { RegionDialog } from "@/components/dialogs/RegionDialog";
import { api, Kind } from "@/lib/api";
import { session } from "@/test/fixtures";

describe("RegionDialog", () => {
  it("saves a new region through api.SetRegion and closes", async () => {
    const user = userEvent.setup();
    const setRegion = vi.spyOn(api, "SetRegion").mockResolvedValue();
    const onClose = vi.fn();
    const s = session({ name: "personal", kind: Kind.KindAWSIAMUser, region: "us-east-1" });
    render(<RegionDialog session={s} onClose={onClose} />);

    expect(screen.getByRole("heading", { name: "Change region" })).toBeInTheDocument();
    const save = screen.getByRole("button", { name: "Save" });
    // Unchanged region: nothing to save.
    expect(save).toBeDisabled();

    await user.click(screen.getByRole("combobox"));
    await user.click(await screen.findByText("eu-west-1"));
    expect(save).toBeEnabled();
    await user.click(save);

    await waitFor(() => expect(setRegion).toHaveBeenCalledWith(s.id, "eu-west-1"));
    expect(onClose).toHaveBeenCalledTimes(1);
  });

  it("renders nothing without a session", () => {
    render(<RegionDialog session={null} onClose={vi.fn()} />);
    expect(screen.queryByRole("dialog")).not.toBeInTheDocument();
  });
});
