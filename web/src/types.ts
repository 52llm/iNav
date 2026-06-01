export interface Bookmark {
  id: number;
  url: string;
  title: string;
  faviconUrl: string;
  summary: string;
  status: "pending" | "tagged" | "failed";
  tags: string[];
}
