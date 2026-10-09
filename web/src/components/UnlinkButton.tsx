"use client";

import { useRouter } from "next/navigation";
import { useState } from "react";
import { createClient } from "@/lib/supabase/client";

export default function UnlinkButton({ id, name }: { id: string; name: string }) {
  const router = useRouter();
  const [confirm, setConfirm] = useState(false);
  async function unlink() {
    if (!confirm) {
      setConfirm(true);
      return;
    }
    await createClient().from("devices").delete().eq("id", id);
    router.refresh();
  }
  return (
    <button className="button" onClick={unlink} aria-label={`Unlink ${name}`}>
      {confirm ? "Click again to unlink" : "Unlink"}
    </button>
  );
}
