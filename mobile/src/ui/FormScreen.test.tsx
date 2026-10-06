import { act, fireEvent, render, screen } from '@testing-library/react-native';
import { Keyboard, KeyboardAvoidingView, Platform, Text, View } from 'react-native';

import { FormScreen } from '@/ui/FormScreen';

type Listener = () => void;

let listeners: Record<string, Listener[]>;
let removed: string[];

beforeEach(() => {
  listeners = {};
  removed = [];
  jest.spyOn(Keyboard, 'addListener').mockImplementation(((event: string, cb: Listener) => {
    (listeners[event] ??= []).push(cb);
    return { remove: () => removed.push(event) };
  }) as unknown as typeof Keyboard.addListener);
});

afterEach(() => {
  jest.restoreAllMocks();
});

async function emit(event: string) {
  await act(async () => (listeners[event] ?? []).forEach((cb) => cb()));
}

async function renderForm(centred?: boolean) {
  await render(
    <FormScreen centred={centred} footer={<Text>Pay</Text>}>
      <Text>Body</Text>
    </FormScreen>,
  );
}

/** The safe-area edge for the bottom: "off" drops the home-indicator inset. */
function footerBottomEdge() {
  return (screen.getByText('Pay').parent?.props.edges as { bottom: string }).bottom;
}

describe('FormScreen', () => {
  it('shows the content and the footer', async () => {
    await renderForm();
    expect(screen.getByText('Body')).toBeOnTheScreen();
    expect(screen.getByText('Pay')).toBeOnTheScreen();
  });

  it('keeps the home-indicator inset under the footer until the keyboard covers it (iOS)', async () => {
    await renderForm();
    expect(footerBottomEdge()).not.toBe('off');
    await emit('keyboardWillShow');
    expect(footerBottomEdge()).toBe('off');
    await emit('keyboardWillHide');
    expect(footerBottomEdge()).not.toBe('off');
  });

  it('listens to the Did events on Android', async () => {
    jest.replaceProperty(Platform, 'OS', 'android');
    await renderForm();
    expect(Object.keys(listeners).sort()).toEqual(['keyboardDidHide', 'keyboardDidShow']);
    await emit('keyboardDidShow');
    expect(footerBottomEdge()).toBe('off');
    await emit('keyboardDidHide');
    expect(footerBottomEdge()).not.toBe('off');
  });

  it('removes its keyboard listeners when it unmounts', async () => {
    await renderForm();
    const added = Object.entries(listeners).flatMap(([event, cbs]) => cbs.map(() => event));
    expect(removed.length).toBeLessThan(added.length);
    await screen.unmount();
    expect(removed.sort()).toEqual(added.sort());
  });

  it('measures its distance from the top of the window and gives it to the keyboard view as the offset', async () => {
    const offsets: unknown[] = [];
    const render0 = KeyboardAvoidingView.prototype.render;
    jest.spyOn(KeyboardAvoidingView.prototype, 'render').mockImplementation(function (this: KeyboardAvoidingView) {
      offsets.push(this.props.keyboardVerticalOffset);
      return render0.call(this);
    });
    jest
      .spyOn(View.prototype as unknown as { measureInWindow: View['measureInWindow'] }, 'measureInWindow')
      .mockImplementation((cb) => cb(0, 88, 400, 700));
    await renderForm();
    expect(offsets.at(-1)).toBe(0);
    await fireEvent(screen.root!, 'layout', { nativeEvent: { layout: { x: 0, y: 0, width: 400, height: 700 } } });
    expect(offsets.at(-1)).toBe(88);
  });

  it('centres the content only when asked', async () => {
    await renderForm();
    const scroll = screen.getByText('Body').parent?.parent;
    expect(scroll?.props.contentContainerStyle).not.toEqual(
      expect.arrayContaining([expect.objectContaining({ justifyContent: 'center' })]),
    );
    await screen.unmount();
    await renderForm(true);
    const centred = screen.getByText('Body').parent?.parent;
    expect(centred?.props.contentContainerStyle).toEqual(
      expect.arrayContaining([expect.objectContaining({ justifyContent: 'center' })]),
    );
  });
});
