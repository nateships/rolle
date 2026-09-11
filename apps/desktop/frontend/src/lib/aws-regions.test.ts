import { describe, expect, it } from "vitest";
import { AWS_REGIONS } from "@/lib/aws-regions";

const all = AWS_REGIONS.flatMap((g) => g.regions);

describe("AWS_REGIONS", () => {
  it("has unique region ids and group names", () => {
    expect(new Set(all.map((r) => r.id)).size).toBe(all.length);
    expect(new Set(AWS_REGIONS.map((g) => g.group)).size).toBe(AWS_REGIONS.length);
  });

  it("has at least one region in every group and a name for each region", () => {
    for (const g of AWS_REGIONS) expect(g.regions.length).toBeGreaterThan(0);
    for (const r of all) expect(r.name).not.toBe("");
  });

  it("uses well-formed region codes", () => {
    for (const r of all) expect(r.id).toMatch(/^[a-z]{2}(-gov)?-[a-z]+-\d$/);
  });

  it("places well-known regions in the expected groups", () => {
    const groupOf = (id: string) => AWS_REGIONS.find((g) => g.regions.some((r) => r.id === id))?.group;
    expect(groupOf("us-east-1")).toBe("United States");
    expect(groupOf("eu-west-1")).toBe("Europe");
    expect(groupOf("ap-southeast-2")).toBe("Asia Pacific");
    expect(groupOf("us-gov-west-1")).toBe("AWS GovCloud (US)");
    expect(all.find((r) => r.id === "us-east-1")?.name).toBe("N. Virginia");
  });
});
