// The miniature dashboard. Built from markup, not a screenshot, so it keeps
// pace with the app and can move. Group rows have no play button.
export type DemoSession = {
  id: string;
  cloud?: "aws" | "azure" | "gcp";
  name: string;
  sub?: string;
  badge?: string;
  profile?: string;
  region?: string;
  group?: boolean;
  fav?: boolean;
  tag?: string;
  active?: number;
};
export const sessions: DemoSession[] = [
  { id: "acme-prod", group: true, cloud: "aws", name: "Acme Prod", sub: "123456789012 · 2 roles · 1 active" },
  { id: "admin", name: "AdministratorAccess", profile: "default", region: "us-east-1", fav: true, active: 2814 },
  { id: "readonly", name: "ReadOnlyAccess", profile: "default", region: "us-east-1" },
  { id: "personal", cloud: "aws", name: "personal", badge: "IAM user", sub: "us-west-2", profile: "personal", region: "us-west-2" },
  {
    id: "prod-admin",
    cloud: "aws",
    name: "prod-admin",
    badge: "Assume role",
    sub: "arn:aws:iam::123456789012:role/Admin",
    profile: "default",
    region: "eu-west-1",
  },
  { id: "contoso", cloud: "azure", name: "Contoso Production", sub: "0f1e2d3c-4b5a-6978-8a9b-0c1d2e3f4a5b" },
  { id: "data-platform", cloud: "gcp", name: "data-platform", sub: "data-platform-4821" },
  { id: "deployer", cloud: "gcp", name: "deployer", tag: "Production", sub: "data-platform-4821 · deployer@data-pl…", fav: true },
];
