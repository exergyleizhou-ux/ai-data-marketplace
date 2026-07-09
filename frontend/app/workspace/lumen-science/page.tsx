import { redirect } from "next/navigation";

/** Deep link → unified workbench Science bridge tab */
export default function LumenScienceWorkspaceRedirect() {
  redirect("/workspace?tab=science");
}
