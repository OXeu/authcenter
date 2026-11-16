import { computed, onMounted, onUnmounted, ref } from 'vue';
const storageKey = 'cc_theme';
function validTheme(value) {
    return value === 'light' || value === 'dark' ? value : null;
}
function savedTheme() {
    try {
        return validTheme(window.localStorage.getItem(storageKey));
    }
    catch {
        return null;
    }
}
/** Manages the saved theme and follows system changes until a choice is made. */
export function useTheme() {
    const preference = ref(savedTheme());
    const media = window.matchMedia?.('(prefers-color-scheme: dark)');
    const systemDark = ref(media?.matches ?? false);
    const theme = computed(() => preference.value ?? (systemDark.value ? 'dark' : 'light'));
    function systemChanged(event) {
        systemDark.value = event.matches;
    }
    function storageChanged(event) {
        if (event.key === storageKey || event.key === null)
            preference.value = savedTheme();
    }
    function toggleTheme() {
        preference.value = theme.value === 'dark' ? 'light' : 'dark';
        try {
            window.localStorage.setItem(storageKey, preference.value);
        }
        catch {
            // Browsers may deny storage; the current page still keeps the chosen theme.
        }
    }
    onMounted(() => {
        media?.addEventListener('change', systemChanged);
        window.addEventListener('storage', storageChanged);
    });
    onUnmounted(() => {
        media?.removeEventListener('change', systemChanged);
        window.removeEventListener('storage', storageChanged);
    });
    return { theme, toggleTheme };
}
