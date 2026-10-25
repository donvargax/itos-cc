export async function loadInvoices(id: string) {
  const res = await fetch(`/api/invoices/${id}`);
  return res.json();
}
