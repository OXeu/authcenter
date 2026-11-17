import { shallowRef } from 'vue'

export const toastTarget = shallowRef<HTMLElement | string>('body')

export function attachToasts(dialog: HTMLDialogElement): () => void {
  toastTarget.value = dialog
  return () => {
    if (toastTarget.value === dialog) toastTarget.value = 'body'
  }
}
