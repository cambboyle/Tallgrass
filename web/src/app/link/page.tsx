import { Suspense } from "react";
import LinkForm from "@/components/LinkForm";

export const metadata = { title: "Link a device" };

export default function LinkPage() {
  return (
    <main style={{ maxWidth: 520 }}>
      <div className="stack">
        <h1>Link a device</h1>
        <p className="muted">
          On your Tallgrass device, open the dashboard&apos;s Account page and choose <strong>Link this device</strong>. It shows a
          code; enter it here. Codes last 10 minutes.
        </p>
      </div>
      <Suspense>
        <LinkForm />
      </Suspense>
    </main>
  );
}
