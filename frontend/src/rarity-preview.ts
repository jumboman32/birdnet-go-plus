import { mount } from 'svelte';
import './styles/tailwind.css';
import './styles/schemes.css';
import LocalRarityPreview from './lib/desktop/views/LocalRarityPreview.svelte';

const target = document.getElementById('app');
if (!import.meta.env.DEV || import.meta.env.MODE !== 'rarity') {
  throw new Error('Rarity preview requires npm run dev:rarity');
}
if (!target) throw new Error('Missing rarity preview mount point');
mount(LocalRarityPreview, { target });
