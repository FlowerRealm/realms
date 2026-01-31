export function closeModalById(id: string) {
  const root = document.getElementById(id);
  root?.querySelector<HTMLButtonElement>('button[data-bs-dismiss="modal"]')?.click();
}

