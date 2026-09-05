import type { MetadataRoute } from "next";

// The PWA remains a thin server client. Installing it does not grant or require
// any direct access to printers on the phone.
export default function manifest(): MetadataRoute.Manifest {
  return {
    name: "CommerceOps Quick Print",
    short_name: "Quick Print",
    description: "Queue authorized CommerceOps print jobs from warehouse devices.",
    start_url: "/",
    display: "standalone",
    background_color: "#f6f8f6",
    theme_color: "#32734a",
  };
}
