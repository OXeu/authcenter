import { beforeEach, afterEach, describe, it, expect, vi } from 'vite-plus/test';
import { defineComponent, h, nextTick } from 'vue';
import { mount } from '@vue/test-utils';
import { useTheme } from '../src/theme.js';
let media;
let mounted;
const Harness = defineComponent({
    setup() {
        const { theme, toggleTheme } = useTheme();
        return () => h('button', { 'data-theme': theme.value, onClick: toggleTheme }, theme.value);
    },
});
beforeEach(() => {
    localStorage.clear();
    media = { matches: false, addEventListener: vi.fn(), removeEventListener: vi.fn() };
    vi.stubGlobal('matchMedia', vi.fn(() => media));
});
afterEach(() => {
    mounted?.unmount();
    vi.restoreAllMocks();
    vi.unstubAllGlobals();
});
/** @returns {ReturnType<typeof mount>} */
function start() {
    mounted = mount(Harness);
    return mounted;
}
describe('theme preference', () => {
    it('follows system changes until the user chooses a theme', async () => {
        const wrapper = start();
        expect(wrapper.attributes('data-theme')).toBe('light');
        const changed = media.addEventListener.mock.calls[0][1];
        changed({ matches: true });
        await nextTick();
        expect(wrapper.attributes('data-theme')).toBe('dark');
        await wrapper.trigger('click');
        expect(localStorage.getItem('cc_theme')).toBe('light');
        changed({ matches: true });
        await nextTick();
        expect(wrapper.attributes('data-theme')).toBe('light');
    });
    it('restores the saved preference on the next page mount', async () => {
        const wrapper = start();
        await wrapper.trigger('click');
        wrapper.unmount();
        expect(start().attributes('data-theme')).toBe('dark');
    });
    it('ignores invalid stored values and uses the initial system setting', () => {
        localStorage.setItem('cc_theme', 'invalid');
        media.matches = true;
        expect(start().attributes('data-theme')).toBe('dark');
    });
    it('syncs changes from other tabs and falls back to the system after clearing', async () => {
        const wrapper = start();
        localStorage.setItem('cc_theme', 'dark');
        window.dispatchEvent(new StorageEvent('storage', { key: 'cc_theme' }));
        await nextTick();
        expect(wrapper.attributes('data-theme')).toBe('dark');
        localStorage.clear();
        window.dispatchEvent(new StorageEvent('storage', { key: null }));
        await nextTick();
        expect(wrapper.attributes('data-theme')).toBe('light');
    });
    it('still switches when browser storage is unavailable', async () => {
        vi.spyOn(Storage.prototype, 'getItem').mockImplementation(() => {
            throw new Error('denied');
        });
        vi.spyOn(Storage.prototype, 'setItem').mockImplementation(() => {
            throw new Error('denied');
        });
        const wrapper = start();
        await wrapper.trigger('click');
        expect(wrapper.attributes('data-theme')).toBe('dark');
    });
    it('releases system and storage listeners on unmount', () => {
        const remove = vi.spyOn(window, 'removeEventListener');
        start().unmount();
        expect(media.removeEventListener).toHaveBeenCalledWith('change', media.addEventListener.mock.calls[0][1]);
        expect(remove).toHaveBeenCalledWith('storage', expect.any(Function));
    });
});
