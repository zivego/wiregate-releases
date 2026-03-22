import { useTheme } from "./ThemeContext";
import { uiTheme } from "./uiTheme";

export function ThemeToggleButton() {
  const { theme, saving, toggleTheme } = useTheme();

  return (
    <button
      onClick={() => void toggleTheme()}
      disabled={saving}
      style={styles.button}
      title="Toggle personal theme preference"
    >
      {saving ? "Saving..." : theme === "light" ? "Dark theme" : "Light theme"}
    </button>
  );
}

const styles: Record<string, React.CSSProperties> = {
  button: {
    background: "transparent",
    color: uiTheme.headerText,
    border: `1px solid ${uiTheme.border}`,
    padding: "0.45rem 0.8rem",
    borderRadius: 6,
    cursor: "pointer",
    fontSize: "0.85rem",
  },
};
