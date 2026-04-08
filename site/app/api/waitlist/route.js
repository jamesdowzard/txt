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
  const submitUrl = process.env.WAITLIST_SUBMIT_URL;

  if (website) {
    return NextResponse.json({ ok: true, message: "Thanks." });
  }

  if (!isValidEmail(email)) {
    return NextResponse.json({ error: "Enter a valid email address." }, { status: 400 });
  }

  if (!allowedInterests.has(interest)) {
    return NextResponse.json({ error: "Invalid interest selection." }, { status: 400 });
  }

  if (!submitUrl) {
    console.error("WAITLIST_SUBMIT_URL is not configured.");
    return NextResponse.json(
      { error: "Updates are not configured right now. Email max@maxghenis.com instead." },
      { status: 503 }
    );
  }

  const formBody = new URLSearchParams();
  formBody.set("email", email);
  formBody.set("interest", interest || "general");
  formBody.set("source", "openmessage.ai");
  formBody.set("_subject", `OpenMessage updates signup: ${interest || "general"}`);
  formBody.set("_captcha", "false");
  formBody.set("_template", "table");

  let response;
  let responseText = "";

  try {
    response = await fetch(submitUrl, {
      method: "POST",
      headers: {
        Accept: "application/json",
        "Content-Type": "application/x-www-form-urlencoded",
        Origin: "https://openmessage.ai",
        Referer: "https://openmessage.ai/"
      },
      body: formBody.toString(),
      cache: "no-store"
    });
  } catch (error) {
    console.error("Waitlist submit failed:", error);
    return NextResponse.json(
      { error: "Could not save your email right now. Try again shortly." },
      { status: 502 }
    );
  }

  responseText = await response.text().catch(() => "");

  if (!response.ok) {
    console.error("Waitlist submit rejected:", response.status, responseText);
    return NextResponse.json(
      { error: "Could not save your email right now. Try again shortly." },
      { status: 502 }
    );
  }

  let responseData = null;

  try {
    responseData = responseText ? JSON.parse(responseText) : null;
  } catch {
    responseData = null;
  }

  if (responseData?.success === "false") {
    const message = `${responseData?.message || ""}`;
    console.error("Waitlist submit failed:", message);

    if (/activation/i.test(message)) {
      return NextResponse.json(
        { error: "Updates signup is being activated right now. Try again in a few minutes." },
        { status: 503 }
      );
    }

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
