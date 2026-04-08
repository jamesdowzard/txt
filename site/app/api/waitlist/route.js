import { put } from "@vercel/blob";
import { NextResponse } from "next/server";

const allowedInterests = new Set(["", "mac-app", "whatsapp", "signal", "mcp", "general"]);

function isValidEmail(value) {
  return /^[^\s@]+@[^\s@]+\.[^\s@]+$/.test(value);
}

export async function POST(request) {
  let payload;

  try {
    payload = await request.json();
  } catch {
    return NextResponse.json({ error: "Invalid request body." }, { status: 400 });
  }

  const email = `${payload?.email || ""}`.trim().toLowerCase();
  const interest = `${payload?.interest || ""}`.trim();
  const website = `${payload?.website || ""}`.trim();
  const blobToken = process.env.BLOB_READ_WRITE_TOKEN;

  if (website) {
    return NextResponse.json({ ok: true, message: "Thanks." });
  }

  if (!isValidEmail(email)) {
    return NextResponse.json({ error: "Enter a valid email address." }, { status: 400 });
  }

  if (!allowedInterests.has(interest)) {
    return NextResponse.json({ error: "Invalid interest selection." }, { status: 400 });
  }

  if (!blobToken) {
    console.error("BLOB_READ_WRITE_TOKEN is not configured.");
    return NextResponse.json(
      { error: "Updates are not configured right now. Email max@maxghenis.com instead." },
      { status: 503 }
    );
  }

  try {
    const submittedAt = new Date().toISOString();
    const pathname = `waitlist/${submittedAt.replaceAll(":", "-")}-${crypto.randomUUID()}.json`;

    await put(
      pathname,
      JSON.stringify(
        {
          email,
          interest: interest || "general",
          submittedAt,
          source: "openmessage.ai"
        },
        null,
        2
      ),
      {
        access: "private",
        addRandomSuffix: false,
        contentType: "application/json"
      }
    );
  } catch (error) {
    console.error("Waitlist submit failed:", error);
    return NextResponse.json(
      { error: "Could not save your email right now. Try again shortly." },
      { status: 502 }
    );
  }

  return NextResponse.json({
    ok: true,
    message: "Thanks. I’ll send product updates when there’s something real to share."
  });
}
