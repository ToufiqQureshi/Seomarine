const CHECKOUT_SCRIPT = "https://checkout.razorpay.com/v1/checkout.js";

interface RazorpayOptions {
  key: string;
  subscription_id: string;
  name: string;
  description: string;
  theme: { color: string };
  handler: () => void;
  modal: { ondismiss: () => void };
}

declare global {
  interface Window {
    Razorpay?: new (options: RazorpayOptions) => { open: () => void };
  }
}

function loadCheckoutScript(): Promise<void> {
  if (window.Razorpay) return Promise.resolve();
  return new Promise((resolve, reject) => {
    const script = document.createElement("script");
    script.src = CHECKOUT_SCRIPT;
    script.async = true;
    script.addEventListener("load", () => resolve());
    script.addEventListener("error", () => {
      // Drop the failed tag so the next attempt loads the script again.
      script.remove();
      reject(
        new Error(
          "Couldn't load Razorpay. Check your connection or ad blocker and try again.",
        ),
      );
    });
    document.head.appendChild(script);
  });
}

/**
 * Opens Razorpay Checkout for a subscription. Resolves `true` once the
 * payment succeeds and `false` when the customer closes the window.
 */
export async function openRazorpayCheckout(session: {
  keyId: string;
  subscriptionId: string;
}): Promise<boolean> {
  await loadCheckoutScript();
  const Razorpay = window.Razorpay;
  if (!Razorpay)
    throw new Error("Razorpay didn't start. Reload and try again.");
  return new Promise((resolve) => {
    new Razorpay({
      key: session.keyId,
      subscription_id: session.subscriptionId,
      name: "Seomarine",
      description: "Seomarine Pro, billed monthly",
      theme: { color: "#0f766e" },
      handler: () => resolve(true),
      modal: { ondismiss: () => resolve(false) },
    }).open();
  });
}
