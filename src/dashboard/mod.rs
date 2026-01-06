//! TUI Dashboard for real-time progress monitoring

use ratatui::{
    backend::CrosstermBackend,
    layout::{Alignment, Constraint, Direction, Layout, Rect},
    style::{Color, Modifier, Style},
    text::{Line, Span, Text},
    widgets::{Block, Borders, Gauge, Paragraph, Wrap},
    Frame, Terminal,
};
use crossterm::{
    event::{self, DisableMouseCapture, EnableMouseCapture, Event, KeyCode},
    execute,
    terminal::{disable_raw_mode, enable_raw_mode, EnterAlternateScreen, LeaveAlternateScreen},
};
use std::io;
use std::time::Duration;
use anyhow::Result;
use crate::drover::{ProjectStatus, WorkManifest};

/// Dashboard for displaying real-time progress
pub struct Dashboard {
    terminal: Terminal<CrosstermBackend<io::Stdout>>,
}

impl Dashboard {
    /// Create a new dashboard
    pub fn new() -> Result<Self> {
        enable_raw_mode()?;
        let mut stdout = io::stdout();
        execute!(stdout, EnterAlternateScreen, EnableMouseCapture)?;
        let backend = CrosstermBackend::new(stdout);
        let terminal = Terminal::new(backend)?;

        Ok(Self { terminal })
    }

    /// Update the dashboard with current status
    pub fn update(&mut self, status: &ProjectStatus, manifest: &WorkManifest) -> Result<()> {
        self.terminal.draw(|f| {
            Self::render(f, status, manifest);
        })?;

        // Check for quit event
        if event::poll(Duration::from_millis(100))? {
            if let Event::Key(key) = event::read()? {
                if key.code == KeyCode::Char('q') {
                    std::process::exit(0);
                }
            }
        }

        Ok(())
    }

    fn render(f: &mut Frame, status: &ProjectStatus, manifest: &WorkManifest) {
        let chunks = Layout::default()
            .direction(Direction::Vertical)
            .margin(2)
            .constraints([
                Constraint::Length(3),  // Header
                Constraint::Length(3),  // Progress bar
                Constraint::Length(10), // Stats
                Constraint::Min(0),     // Task list
            ])
            .split(f.area());

        // Header
        let header = vec![
            Line::from(vec![
                Span::styled("🐂 ", Style::default().fg(Color::Yellow)),
                Span::styled("Drover", Style::default().fg(Color::White).add_modifier(Modifier::BOLD)),
                Span::styled(" - No task left behind", Style::default().fg(Color::Gray)),
            ]),
        ];
        let header = Paragraph::new(header)
            .alignment(Alignment::Center);
        f.render_widget(header, chunks[0]);

        // Progress bar
        let progress_label = format!(
            "{:.1}% ({}/{})",
            status.progress,
            status.completed,
            status.total
        );
        let gauge = Gauge::default()
            .block(Block::default().borders(Borders::ALL).title("Progress"))
            .gauge_style(
                Style::default()
                    .fg(Color::Green)
                    .add_modifier(Modifier::BOLD),
            )
            .percent(status.progress as u16)
            .label(progress_label);
        f.render_widget(gauge, chunks[1]);

        // Stats
        let stats = vec![
            Line::from(vec![
                Span::styled("Total:    ", Style::default().fg(Color::Gray)),
                Span::styled(format!("{}", status.total), Style::default().fg(Color::White)),
            ]),
            Line::from(vec![
                Span::styled("Ready:    ", Style::default().fg(Color::Gray)),
                Span::styled(format!("{}", status.ready), Style::default().fg(Color::Cyan)),
            ]),
            Line::from(vec![
                Span::styled("Blocked:  ", Style::default().fg(Color::Gray)),
                Span::styled(format!("{}", status.blocked), Style::default().fg(Color::Yellow)),
            ]),
            Line::from(vec![
                Span::styled("Failed:   ", Style::default().fg(Color::Gray)),
                Span::styled(format!("{}", status.failed), Style::default().fg(Color::Red)),
            ]),
        ];

        let stats_block = Paragraph::new(stats)
            .block(Block::default().borders(Borders::ALL).title("Status"));
        f.render_widget(stats_block, chunks[2]);

        // Epics list
        let epics_text: Vec<Line> = manifest.epics.iter().map(|epic| {
            let status_symbol = if epic.progress >= 100.0 {
                "✓"
            } else if epic.progress > 0.0 {
                "◐"
            } else {
                "○"
            };

            Line::from(vec![
                Span::styled(status_symbol, Style::default().fg(Color::Green)),
                Span::raw(" "),
                Span::styled(&epic.title, Style::default().fg(Color::White)),
                Span::raw(" "),
                Span::styled(
                    format!("({:.0}%)", epic.progress),
                    Style::default().fg(Color::Gray),
                ),
            ])
        }).collect();

        let epics_block = Paragraph::new(epics_text)
            .block(Block::default().borders(Borders::ALL).title("Epics"))
            .wrap(Wrap { trim: true });
        f.render_widget(epics_block, chunks[3]);
    }

    /// Clean up the terminal
    pub fn cleanup(&mut self) -> Result<()> {
        disable_raw_mode()?;
        execute!(
            self.terminal.backend_mut(),
            LeaveAlternateScreen,
            DisableMouseCapture
        )?;
        self.terminal.show_cursor()?;
        Ok(())
    }
}

impl Drop for Dashboard {
    fn drop(&mut self) {
        let _ = self.cleanup();
    }
}

/// Simple non-TUI status printer
pub fn print_status(status: &ProjectStatus, manifest: &WorkManifest) {
    let progress_bar_len = 40;
    let filled = (status.progress / 100.0 * progress_bar_len as f32) as usize;
    let empty = progress_bar_len - filled;

    print!("\x1b[2J\x1b[H"); // Clear screen and move cursor to home

    println!("🐂 Drover - {}\n", manifest.target_description());

    println!("[{}{}] {:.1}% ({}/{})",
        "█".repeat(filled),
        "░".repeat(empty),
        status.progress,
        status.completed,
        status.total
    );

    println!("\nStatus:");
    println!("  Total:   {}", status.total);
    println!("  Ready:   {}", status.ready);
    println!("  Blocked: {}", status.blocked);
    println!("  Failed:  {}", status.failed);

    if !manifest.epics.is_empty() {
        println!("\nEpics:");
        for epic in &manifest.epics {
            let symbol = if epic.progress >= 100.0 { "✓" } else { "○" };
            println!("  {} {} ({:.0}%)", symbol, epic.title, epic.progress);
        }
    }
}
