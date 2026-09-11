/** AWS regions, grouped for the picker. Identity Center portals live in one of these. */
export const AWS_REGIONS: { group: string; regions: { id: string; name: string }[] }[] = [
  {
    group: "United States",
    regions: [
      { id: "us-east-1", name: "N. Virginia" },
      { id: "us-east-2", name: "Ohio" },
      { id: "us-west-1", name: "N. California" },
      { id: "us-west-2", name: "Oregon" },
    ],
  },
  {
    group: "Canada",
    regions: [
      { id: "ca-central-1", name: "Central" },
      { id: "ca-west-1", name: "Calgary" },
    ],
  },
  {
    group: "Europe",
    regions: [
      { id: "eu-west-1", name: "Ireland" },
      { id: "eu-west-2", name: "London" },
      { id: "eu-west-3", name: "Paris" },
      { id: "eu-central-1", name: "Frankfurt" },
      { id: "eu-central-2", name: "Zurich" },
      { id: "eu-north-1", name: "Stockholm" },
      { id: "eu-south-1", name: "Milan" },
      { id: "eu-south-2", name: "Spain" },
    ],
  },
  {
    group: "Asia Pacific",
    regions: [
      { id: "ap-south-1", name: "Mumbai" },
      { id: "ap-south-2", name: "Hyderabad" },
      { id: "ap-northeast-1", name: "Tokyo" },
      { id: "ap-northeast-2", name: "Seoul" },
      { id: "ap-northeast-3", name: "Osaka" },
      { id: "ap-southeast-1", name: "Singapore" },
      { id: "ap-southeast-2", name: "Sydney" },
      { id: "ap-southeast-3", name: "Jakarta" },
      { id: "ap-southeast-4", name: "Melbourne" },
      { id: "ap-east-1", name: "Hong Kong" },
    ],
  },
  {
    group: "Middle East & Africa",
    regions: [
      { id: "me-south-1", name: "Bahrain" },
      { id: "me-central-1", name: "UAE" },
      { id: "il-central-1", name: "Tel Aviv" },
      { id: "af-south-1", name: "Cape Town" },
    ],
  },
  { group: "South America", regions: [{ id: "sa-east-1", name: "São Paulo" }] },
  {
    group: "AWS GovCloud (US)",
    regions: [
      { id: "us-gov-east-1", name: "US-East" },
      { id: "us-gov-west-1", name: "US-West" },
    ],
  },
];
