import { redirect } from "next/navigation";

/** Deep link → unified workbench Lab tab */
export default function LumenLabWorkspaceRedirect() {
  redirect("/workspace?tab=lab");
}
