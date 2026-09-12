import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { toast } from "sonner";
import { describe, expect, it, vi } from "vitest";
import { CommandLineInstall } from "@/components/CommandLine";
import { api, type CLIStatus } from "@/lib/api";

const missing: CLIStatus = { installed: false, path: "", target: "/Applications/rolle.app/rolle", reason: "" };
const linked: CLIStatus = { installed: true, path: "/usr/local/bin/rolle", target: "/Applications/rolle.app/rolle" };

describe("CommandLineInstall", () => {
  it("renders nothing where the command cannot be linked", async () => {
    vi.spyOn(api, "CLIStatus").mockResolvedValue({ installed: false, reason: "unsupported" });
    const { container } = render(<CommandLineInstall />);
    await waitFor(() => expect(api.CLIStatus).toHaveBeenCalled());
    expect(container).toBeEmptyDOMElement();
  });

  it("installs the command and shows where it landed", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CLIStatus").mockResolvedValueOnce(missing).mockResolvedValue(linked);
    const install = vi.spyOn(api, "InstallCLI").mockResolvedValue(linked);
    const success = vi.spyOn(toast, "success");
    render(<CommandLineInstall />);

    expect(await screen.findByText(/adds/i)).toHaveTextContent("Adds rolle to your PATH.");
    await user.click(screen.getByRole("button", { name: /install command/i }));

    await waitFor(() => expect(success).toHaveBeenCalledWith("rolle command installed"));
    expect(install).toHaveBeenCalledTimes(1);
    expect(await screen.findByText("Installed")).toBeInTheDocument();
    expect(screen.getByText("/usr/local/bin/rolle")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /install command/i })).not.toBeInTheDocument();
  });

  it("asks the user to move the app first", async () => {
    vi.spyOn(api, "CLIStatus").mockResolvedValue({ ...missing, reason: "move" });
    render(<CommandLineInstall />);
    expect(await screen.findByText("Move rolle to Applications first.")).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /install command/i })).toBeDisabled();
  });

  it("offers an update when the installed command is another version", async () => {
    vi.spyOn(api, "CLIStatus").mockResolvedValue({ ...linked, reason: "outdated" });
    render(<CommandLineInstall />);
    expect(await screen.findByText(/is from another version/)).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /update command/i })).toBeEnabled();
  });

  it("passes the platform's note along", async () => {
    vi.spyOn(api, "CLIStatus").mockResolvedValue({ ...linked, note: "Open a new terminal to use it." });
    render(<CommandLineInstall />);
    expect(await screen.findByText(/Open a new terminal to use it\./)).toBeInTheDocument();
  });

  it("removes the command from the compact view", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CLIStatus").mockResolvedValueOnce(linked).mockResolvedValue(missing);
    const remove = vi.spyOn(api, "UninstallCLI").mockResolvedValue();
    const success = vi.spyOn(toast, "success");
    render(<CommandLineInstall compact />);

    expect(await screen.findByText("Installed")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Remove" }));

    await waitFor(() => expect(success).toHaveBeenCalledWith("rolle command removed"));
    expect(remove).toHaveBeenCalledTimes(1);
    expect(await screen.findByRole("button", { name: /install command/i })).toBeInTheDocument();
  });

  it("stays quiet when the user cancels the install prompt", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CLIStatus").mockResolvedValue(missing);
    vi.spyOn(api, "InstallCLI").mockRejectedValue(new Error("cancelled"));
    const error = vi.spyOn(toast, "error");
    render(<CommandLineInstall compact />);

    await user.click(await screen.findByRole("button", { name: /install command/i }));
    await waitFor(() => expect(screen.getByRole("button", { name: /install command/i })).toBeEnabled());
    expect(error).not.toHaveBeenCalled();
  });

  it("shows any other install failure", async () => {
    const user = userEvent.setup();
    vi.spyOn(api, "CLIStatus").mockResolvedValue(missing);
    vi.spyOn(api, "InstallCLI").mockRejectedValue(new Error("permission denied"));
    const error = vi.spyOn(toast, "error");
    render(<CommandLineInstall />);

    await user.click(await screen.findByRole("button", { name: /install command/i }));
    await waitFor(() => expect(error).toHaveBeenCalledWith("permission denied"));
  });
});
