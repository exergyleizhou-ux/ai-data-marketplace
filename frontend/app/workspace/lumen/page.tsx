import { redirect } from "next/navigation";

/** Deep link → unified workbench coding tab */
export default function LumenWorkspaceRedirect() {
  redirect("/workspace?tab=coding");
}
