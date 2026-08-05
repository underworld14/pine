import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import AddCardComposer from './AddCardComposer.svelte';

describe('AddCardComposer', () => {
  it('opens the composer, submits a title on Enter, and stays open', async () => {
    const onSubmit = vi.fn();
    render(AddCardComposer, { status: 'todo', onSubmit });

    await fireEvent.click(screen.getByTestId('add-card-todo'));
    const input = screen.getByTestId('add-card-input-todo') as HTMLTextAreaElement;
    await fireEvent.input(input, { target: { value: 'New card' } });
    await fireEvent.keyDown(input, { key: 'Enter' });

    expect(onSubmit).toHaveBeenCalledWith('New card');
    // Still open for rapid entry, cleared.
    expect(screen.getByTestId('add-card-input-todo')).toBeTruthy();
  });

  it('does not submit an empty/whitespace title', async () => {
    const onSubmit = vi.fn();
    render(AddCardComposer, { status: 'todo', onSubmit });
    await fireEvent.click(screen.getByTestId('add-card-todo'));
    const input = screen.getByTestId('add-card-input-todo');
    await fireEvent.input(input, { target: { value: '   ' } });
    await fireEvent.keyDown(input, { key: 'Enter' });
    expect(onSubmit).not.toHaveBeenCalled();
  });

  it('closes on Escape', async () => {
    render(AddCardComposer, { status: 'todo', onSubmit: vi.fn() });
    await fireEvent.click(screen.getByTestId('add-card-todo'));
    const input = screen.getByTestId('add-card-input-todo');
    await fireEvent.keyDown(input, { key: 'Escape' });
    expect(screen.queryByTestId('add-card-input-todo')).toBeNull();
    expect(screen.getByTestId('add-card-todo')).toBeTruthy();
  });
});
